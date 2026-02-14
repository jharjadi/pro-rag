// Package service implements the query pipeline business logic.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
	"github.com/pgvector/pgvector-go"
)

// RetrievalService handles vector and FTS search against the database.
type RetrievalService struct {
	pool *pgxpool.Pool
}

// NewRetrievalService creates a new RetrievalService.
func NewRetrievalService(pool *pgxpool.Pool) *RetrievalService {
	return &RetrievalService{pool: pool}
}

// RetrievalResult holds the results of parallel vector + FTS retrieval.
type RetrievalResult struct {
	VecResults []model.ChunkResult
	FTSResults []model.ChunkResult
}

// Retrieve runs vector search and FTS search in parallel, returning both result sets.
func (s *RetrievalService) Retrieve(ctx context.Context, tenantID, question string, questionEmbedding []float32, kVec, kFTS int) (*RetrievalResult, error) {
	var (
		wg         sync.WaitGroup
		vecResults []model.ChunkResult
		ftsResults []model.ChunkResult
		vecErr     error
		ftsErr     error
	)

	wg.Add(2)

	// Vector search (parallel)
	go func() {
		defer wg.Done()
		vecResults, vecErr = s.vectorSearch(ctx, tenantID, questionEmbedding, kVec)
	}()

	// FTS search (parallel)
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = s.ftsSearch(ctx, tenantID, question, kFTS)
	}()

	wg.Wait()

	if vecErr != nil {
		return nil, fmt.Errorf("vector search: %w", vecErr)
	}
	if ftsErr != nil {
		return nil, fmt.Errorf("FTS search: %w", ftsErr)
	}

	return &RetrievalResult{
		VecResults: vecResults,
		FTSResults: ftsResults,
	}, nil
}

// vectorSearch performs cosine similarity search using pgvector HNSW index.
// Joins chunks → chunk_embeddings → document_versions → documents.
// Filters: tenant_id + is_active = true.
func (s *RetrievalService) vectorSearch(ctx context.Context, tenantID string, embedding []float32, k int) ([]model.ChunkResult, error) {
	query := `
		SELECT
			c.chunk_id,
			c.tenant_id,
			c.doc_version_id,
			d.doc_id,
			d.title,
			dv.version_label,
			c.heading_path,
			c.chunk_type,
			c.text,
			c.token_count,
			c.ordinal,
			1 - (ce.embedding <=> $2) AS vec_score
		FROM chunks c
		JOIN chunk_embeddings ce ON ce.chunk_id = c.chunk_id
		JOIN document_versions dv ON dv.doc_version_id = c.doc_version_id
		JOIN documents d ON d.doc_id = dv.doc_id
		WHERE c.tenant_id = $1
		  AND dv.is_active = true
		ORDER BY ce.embedding <=> $2
		LIMIT $3
	`

	vec := pgvector.NewVector(embedding)
	rows, err := s.pool.Query(ctx, query, tenantID, vec, k)
	if err != nil {
		return nil, fmt.Errorf("vector query: %w", err)
	}
	defer rows.Close()

	var results []model.ChunkResult
	rank := 1
	for rows.Next() {
		var cr model.ChunkResult
		var headingPathJSON []byte

		err := rows.Scan(
			&cr.ChunkID,
			&cr.TenantID,
			&cr.DocVersionID,
			&cr.DocID,
			&cr.Title,
			&cr.VersionLabel,
			&headingPathJSON,
			&cr.ChunkType,
			&cr.Text,
			&cr.TokenCount,
			&cr.Ordinal,
			&cr.VecScore,
		)
		if err != nil {
			return nil, fmt.Errorf("scan vector row: %w", err)
		}

		// Parse heading_path JSONB array
		if headingPathJSON != nil {
			if err := json.Unmarshal(headingPathJSON, &cr.HeadingPath); err != nil {
				slog.Warn("failed to parse heading_path", "chunk_id", cr.ChunkID, "error", err)
			}
		}

		cr.VecRank = rank
		rank++
		results = append(results, cr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector rows iteration: %w", err)
	}

	return results, nil
}

// ftsSearch performs full-text search using websearch_to_tsquery and ts_rank_cd.
// Joins chunks → chunk_fts → document_versions → documents.
// Filters: tenant_id + is_active = true.
func (s *RetrievalService) ftsSearch(ctx context.Context, tenantID, question string, k int) ([]model.ChunkResult, error) {
	query := `
		SELECT
			c.chunk_id,
			c.tenant_id,
			c.doc_version_id,
			d.doc_id,
			d.title,
			dv.version_label,
			c.heading_path,
			c.chunk_type,
			c.text,
			c.token_count,
			c.ordinal,
			ts_rank_cd(cf.tsv, websearch_to_tsquery('english', $2)) AS fts_score
		FROM chunks c
		JOIN chunk_fts cf ON cf.chunk_id = c.chunk_id
		JOIN document_versions dv ON dv.doc_version_id = c.doc_version_id
		JOIN documents d ON d.doc_id = dv.doc_id
		WHERE c.tenant_id = $1
		  AND dv.is_active = true
		  AND cf.tsv @@ websearch_to_tsquery('english', $2)
		ORDER BY ts_rank_cd(cf.tsv, websearch_to_tsquery('english', $2)) DESC
		LIMIT $3
	`

	rows, err := s.pool.Query(ctx, query, tenantID, question, k)
	if err != nil {
		return nil, fmt.Errorf("FTS query: %w", err)
	}
	defer rows.Close()

	var results []model.ChunkResult
	rank := 1
	for rows.Next() {
		var cr model.ChunkResult
		var headingPathJSON []byte

		err := rows.Scan(
			&cr.ChunkID,
			&cr.TenantID,
			&cr.DocVersionID,
			&cr.DocID,
			&cr.Title,
			&cr.VersionLabel,
			&headingPathJSON,
			&cr.ChunkType,
			&cr.Text,
			&cr.TokenCount,
			&cr.Ordinal,
			&cr.FTSScore,
		)
		if err != nil {
			return nil, fmt.Errorf("scan FTS row: %w", err)
		}

		// Parse heading_path JSONB array
		if headingPathJSON != nil {
			if err := json.Unmarshal(headingPathJSON, &cr.HeadingPath); err != nil {
				slog.Warn("failed to parse heading_path", "chunk_id", cr.ChunkID, "error", err)
			}
		}

		cr.FTSRank = rank
		rank++
		results = append(results, cr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FTS rows iteration: %w", err)
	}

	return results, nil
}
