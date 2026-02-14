#!/usr/bin/env python3
"""Evaluation harness for pro-rag.

Supports two modes:
  - retrieval: Runs vector search + FTS + RRF merge directly against the DB (no LLM).
  - full:      Calls POST /v1/query end-to-end through the running API.

Both modes compute: Hit@K, MRR, abstain rate, latency.
Full mode additionally captures: end-to-end latency, stage timings, LLM token usage.

Usage:
    # Retrieval-only (Phase 4a — default)
    python eval/run_eval.py --mode retrieval

    # Full pipeline (Phase 6.1)
    python eval/run_eval.py --mode full --api-url http://localhost:8000

    # Full pipeline comparison: with vs without rerank
    python eval/run_eval.py --mode full --api-url http://localhost:8000 --output eval/results_full.csv

Spec reference: §11, implementation-plan Phase 4a.3 + Phase 6.1
"""

from __future__ import annotations

import argparse
import csv
import json
import logging
import os
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import requests

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s — %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S",
    stream=sys.stderr,
)
logger = logging.getLogger("eval")

# ── Configuration ────────────────────────────────────────

DEFAULT_DATABASE_URL = os.environ.get(
    "DATABASE_URL",
    "postgres://prorag:prorag_dev@localhost:5433/prorag?sslmode=disable",
)
DEFAULT_TENANT_ID = os.environ.get(
    "TENANT_ID",
    "00000000-0000-0000-0000-000000000001",
)
DEFAULT_EMBEDDING_MODEL = os.environ.get("EMBEDDING_MODEL", "BAAI/bge-base-en-v1.5")
DEFAULT_K_VEC = int(os.environ.get("K_VEC", "50"))
DEFAULT_K_FTS = int(os.environ.get("K_FTS", "50"))
DEFAULT_RRF_K = int(os.environ.get("RRF_K", "60"))
DEFAULT_EVAL_K = int(os.environ.get("EVAL_K", "10"))  # Hit@K evaluation depth
DEFAULT_API_URL = os.environ.get("API_URL", "http://localhost:8000")


# ── Data classes ─────────────────────────────────────────

@dataclass
class EvalQuestion:
    question: str
    expected_doc_title: str
    expected_abstain: bool = False


@dataclass
class RetrievalResult:
    chunk_id: str
    doc_title: str
    score: float
    source: str  # "vec", "fts", or "rrf"


@dataclass
class EvalResult:
    """Result for a single evaluated question (works for both modes)."""
    question: str
    expected_doc_title: str
    expected_abstain: bool
    mode: str = "retrieval"
    hit_at_k: bool = False
    reciprocal_rank: float = 0.0
    abstained: bool = False
    num_vec_results: int = 0
    num_fts_results: int = 0
    num_rrf_results: int = 0
    top_doc_title: str = ""
    latency_vec_ms: float = 0.0
    latency_fts_ms: float = 0.0
    latency_merge_ms: float = 0.0
    latency_total_ms: float = 0.0
    # Full pipeline fields
    answer: str = ""
    num_citations: int = 0
    citation_doc_titles: str = ""  # comma-separated
    latency_rerank_ms: float = 0.0
    latency_llm_ms: float = 0.0
    latency_e2e_ms: float = 0.0
    reranker_used: bool = False
    reranker_skipped: bool = False
    llm_prompt_tokens: int = 0
    llm_completion_tokens: int = 0
    http_status: int = 0
    error: str = ""


# ── Database queries (retrieval mode) ────────────────────

def _lazy_import_ingest():
    """Lazily import ingest modules (only needed for retrieval mode)."""
    sys.path.insert(0, str(Path(__file__).parent.parent / "ingest"))
    from ingest.embed.embedder import embed_chunks  # noqa: E402
    from ingest.chunk.chunker import Chunk  # noqa: E402
    return embed_chunks, Chunk


def vector_search(
    conn,
    tenant_id: str,
    query_embedding: list[float],
    k: int = 50,
) -> list[RetrievalResult]:
    """Run vector similarity search against chunk_embeddings."""
    embedding_str = "[" + ",".join(str(v) for v in query_embedding) + "]"

    sql = """
        SELECT c.chunk_id, d.title,
               1 - (ce.embedding <=> %s::vector) AS cosine_similarity
        FROM chunk_embeddings ce
        JOIN chunks c ON c.chunk_id = ce.chunk_id
        JOIN document_versions dv ON dv.doc_version_id = c.doc_version_id
        JOIN documents d ON d.doc_id = dv.doc_id
        WHERE ce.tenant_id = %s
          AND dv.tenant_id = %s
          AND dv.is_active = true
        ORDER BY ce.embedding <=> %s::vector
        LIMIT %s
    """

    with conn.cursor() as cur:
        cur.execute(sql, (embedding_str, tenant_id, tenant_id, embedding_str, k))
        rows = cur.fetchall()

    return [
        RetrievalResult(
            chunk_id=str(row[0]),
            doc_title=row[1],
            score=float(row[2]),
            source="vec",
        )
        for row in rows
    ]


def fts_search(
    conn,
    tenant_id: str,
    query: str,
    k: int = 50,
) -> list[RetrievalResult]:
    """Run full-text search against chunk_fts."""
    sql = """
        SELECT c.chunk_id, d.title,
               ts_rank_cd(cf.tsv, websearch_to_tsquery('english', %s)) AS rank
        FROM chunk_fts cf
        JOIN chunks c ON c.chunk_id = cf.chunk_id
        JOIN document_versions dv ON dv.doc_version_id = c.doc_version_id
        JOIN documents d ON d.doc_id = dv.doc_id
        WHERE cf.tenant_id = %s
          AND dv.tenant_id = %s
          AND dv.is_active = true
          AND cf.tsv @@ websearch_to_tsquery('english', %s)
        ORDER BY rank DESC
        LIMIT %s
    """

    with conn.cursor() as cur:
        cur.execute(sql, (query, tenant_id, tenant_id, query, k))
        rows = cur.fetchall()

    return [
        RetrievalResult(
            chunk_id=str(row[0]),
            doc_title=row[1],
            score=float(row[2]),
            source="fts",
        )
        for row in rows
    ]


def rrf_merge(
    vec_results: list[RetrievalResult],
    fts_results: list[RetrievalResult],
    k: int = 60,
) -> list[RetrievalResult]:
    """Reciprocal Rank Fusion merge of vector and FTS results."""
    scores: dict[str, float] = {}
    titles: dict[str, str] = {}

    for rank, result in enumerate(vec_results):
        scores[result.chunk_id] = scores.get(result.chunk_id, 0.0) + 1.0 / (k + rank + 1)
        titles[result.chunk_id] = result.doc_title

    for rank, result in enumerate(fts_results):
        scores[result.chunk_id] = scores.get(result.chunk_id, 0.0) + 1.0 / (k + rank + 1)
        titles[result.chunk_id] = result.doc_title

    sorted_chunks = sorted(scores.items(), key=lambda x: x[1], reverse=True)

    return [
        RetrievalResult(
            chunk_id=chunk_id,
            doc_title=titles[chunk_id],
            score=score,
            source="rrf",
        )
        for chunk_id, score in sorted_chunks
    ]


# ── Retrieval-mode evaluation ────────────────────────────

def evaluate_question_retrieval(
    conn,
    question: EvalQuestion,
    query_embedding: list[float],
    tenant_id: str,
    eval_k: int,
    k_vec: int,
    k_fts: int,
    rrf_k: int,
) -> EvalResult:
    """Evaluate a single question against the retrieval pipeline (no LLM)."""
    result = EvalResult(
        question=question.question,
        expected_doc_title=question.expected_doc_title,
        expected_abstain=question.expected_abstain,
        mode="retrieval",
    )

    total_start = time.perf_counter()

    # Vector search
    vec_start = time.perf_counter()
    vec_results = vector_search(conn, tenant_id, query_embedding, k=k_vec)
    result.latency_vec_ms = (time.perf_counter() - vec_start) * 1000
    result.num_vec_results = len(vec_results)

    # FTS search
    fts_start = time.perf_counter()
    fts_results = fts_search(conn, tenant_id, question.question, k=k_fts)
    result.latency_fts_ms = (time.perf_counter() - fts_start) * 1000
    result.num_fts_results = len(fts_results)

    # Zero candidates check
    if not vec_results and not fts_results:
        result.abstained = True
        result.latency_total_ms = (time.perf_counter() - total_start) * 1000
        return result

    # RRF merge
    merge_start = time.perf_counter()
    rrf_results = rrf_merge(vec_results, fts_results, k=rrf_k)
    result.latency_merge_ms = (time.perf_counter() - merge_start) * 1000
    result.num_rrf_results = len(rrf_results)

    # Evaluate Hit@K and MRR (document-level matching)
    top_k = rrf_results[:eval_k]
    if top_k:
        result.top_doc_title = top_k[0].doc_title

    for rank, rr in enumerate(top_k):
        if rr.doc_title == question.expected_doc_title:
            result.hit_at_k = True
            result.reciprocal_rank = 1.0 / (rank + 1)
            break

    result.latency_total_ms = (time.perf_counter() - total_start) * 1000
    return result


def run_retrieval_eval(
    questions: list[EvalQuestion],
    database_url: str,
    tenant_id: str,
    embedding_model: str,
    eval_k: int,
    k_vec: int,
    k_fts: int,
    rrf_k: int,
) -> list[EvalResult]:
    """Run retrieval-only evaluation (Phase 4a)."""
    import psycopg2
    import psycopg2.extras

    embed_chunks, Chunk = _lazy_import_ingest()

    # Connect to DB
    psycopg2.extras.register_uuid()
    conn = psycopg2.connect(database_url)
    logger.info("Connected to database")

    # Pre-embed all questions
    logger.info("Embedding %d questions...", len(questions))
    embed_start = time.perf_counter()
    fake_chunks = [
        Chunk(text=q.question, heading_path=[], chunk_type="query", ordinal=0, token_count=0)
        for q in questions
    ]
    all_embeddings = embed_chunks(fake_chunks, model_name=embedding_model, batch_size=256)
    embed_time = (time.perf_counter() - embed_start) * 1000
    logger.info("Embedded %d questions in %.1f ms", len(questions), embed_time)

    # Evaluate each question
    results: list[EvalResult] = []
    for i, (question, embedding) in enumerate(zip(questions, all_embeddings)):
        result = evaluate_question_retrieval(
            conn, question, embedding, tenant_id, eval_k, k_vec, k_fts, rrf_k,
        )
        results.append(result)
        status = "HIT" if result.hit_at_k else ("ABSTAIN" if result.abstained else "MISS")
        logger.info(
            "[%d/%d] %s | Q: %.60s... | Expected: %s | Got: %s | RR: %.2f | %.1f ms",
            i + 1, len(questions), status,
            question.question, question.expected_doc_title,
            result.top_doc_title or "(none)", result.reciprocal_rank,
            result.latency_total_ms,
        )

    conn.close()
    return results


# ── Full pipeline evaluation ─────────────────────────────

def evaluate_question_full(
    question: EvalQuestion,
    api_url: str,
    tenant_id: str,
    eval_k: int,
) -> EvalResult:
    """Evaluate a single question via the full API pipeline (POST /v1/query).

    Calls the running core-api-go with debug=true to capture stage timings.
    Determines Hit@K by checking if any citation's title matches expected_doc_title.
    """
    result = EvalResult(
        question=question.question,
        expected_doc_title=question.expected_doc_title,
        expected_abstain=question.expected_abstain,
        mode="full",
    )

    payload = {
        "tenant_id": tenant_id,
        "question": question.question,
        "top_k": eval_k,
        "debug": True,
    }

    e2e_start = time.perf_counter()
    try:
        resp = requests.post(
            f"{api_url}/v1/query",
            json=payload,
            headers={"Content-Type": "application/json"},
            timeout=60,
        )
        result.latency_e2e_ms = (time.perf_counter() - e2e_start) * 1000
        result.http_status = resp.status_code

        if resp.status_code != 200:
            result.error = f"HTTP {resp.status_code}: {resp.text[:200]}"
            return result

        data = resp.json()
    except requests.RequestException as e:
        result.latency_e2e_ms = (time.perf_counter() - e2e_start) * 1000
        result.error = str(e)
        return result

    # Parse response
    result.answer = data.get("answer", "")
    result.abstained = data.get("abstained", False)

    citations = data.get("citations") or []
    result.num_citations = len(citations)

    # Collect unique doc titles from citations
    citation_titles = list(dict.fromkeys(c.get("title", "") for c in citations))
    result.citation_doc_titles = ", ".join(citation_titles)

    if citation_titles:
        result.top_doc_title = citation_titles[0]

    # Parse debug info for stage timings
    debug = data.get("debug") or {}
    result.num_vec_results = debug.get("vec_candidates", 0)
    result.num_fts_results = debug.get("fts_candidates", 0)
    result.num_rrf_results = debug.get("merged_candidates", 0)
    result.reranker_used = debug.get("reranker_used", False)
    result.reranker_skipped = debug.get("reranker_skipped", False)

    # Hit@K: check if expected doc title appears in any citation
    # For full pipeline, "hit" means the expected document was cited in the answer
    if not result.abstained:
        for rank, title in enumerate(citation_titles):
            if title == question.expected_doc_title:
                result.hit_at_k = True
                result.reciprocal_rank = 1.0 / (rank + 1)
                break

    return result


def run_full_eval(
    questions: list[EvalQuestion],
    api_url: str,
    tenant_id: str,
    eval_k: int,
) -> list[EvalResult]:
    """Run full pipeline evaluation (Phase 6.1).

    Calls POST /v1/query for each question and evaluates the response.
    """
    # Verify API is reachable
    try:
        health = requests.get(f"{api_url}/health", timeout=5)
        if health.status_code != 200:
            logger.error("API health check failed: HTTP %d", health.status_code)
            sys.exit(1)
        logger.info("API health check passed (%s)", api_url)
    except requests.RequestException as e:
        logger.error("Cannot reach API at %s: %s", api_url, e)
        logger.error("Is the stack running? Try: docker compose up -d")
        sys.exit(1)

    results: list[EvalResult] = []
    for i, question in enumerate(questions):
        result = evaluate_question_full(question, api_url, tenant_id, eval_k)
        results.append(result)

        if result.error:
            status = "ERROR"
        elif result.hit_at_k:
            status = "HIT"
        elif result.abstained:
            status = "ABSTAIN"
        else:
            status = "MISS"

        logger.info(
            "[%d/%d] %s | Q: %.50s... | Expected: %s | Cited: %s | RR: %.2f | %.0f ms",
            i + 1, len(questions), status,
            question.question, question.expected_doc_title,
            result.top_doc_title or "(none)", result.reciprocal_rank,
            result.latency_e2e_ms,
        )

    return results


# ── Shared output / metrics ──────────────────────────────

def load_questions(path: str) -> list[EvalQuestion]:
    """Load evaluation questions from JSONL file."""
    questions = []
    with open(path) as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                data = json.loads(line)
                questions.append(EvalQuestion(
                    question=data["question"],
                    expected_doc_title=data["expected_doc_title"],
                    expected_abstain=data.get("expected_abstain", False),
                ))
            except (json.JSONDecodeError, KeyError) as e:
                logger.warning("Skipping invalid line %d: %s", line_num, e)
    return questions


def compute_and_print_metrics(
    results: list[EvalResult],
    eval_k: int,
    mode: str,
) -> dict:
    """Compute aggregate metrics and print summary. Returns summary dict."""
    total = len(results)
    hits = sum(1 for r in results if r.hit_at_k)
    abstains = sum(1 for r in results if r.abstained)
    errors = sum(1 for r in results if r.error)
    non_abstain = [r for r in results if not r.abstained and not r.error]

    hit_at_k = hits / total if total > 0 else 0.0
    mrr = sum(r.reciprocal_rank for r in results) / total if total > 0 else 0.0
    abstain_rate = abstains / total if total > 0 else 0.0
    error_rate = errors / total if total > 0 else 0.0

    print("\n" + "=" * 70)
    mode_label = "RETRIEVAL-ONLY" if mode == "retrieval" else "FULL PIPELINE"
    print(f"{mode_label} EVALUATION RESULTS")
    print("=" * 70)
    print(f"  Questions:      {total}")
    print(f"  Hit@{eval_k}:         {hits}/{total} = {hit_at_k:.3f}")
    print(f"  MRR:            {mrr:.3f}")
    print(f"  Abstain rate:   {abstains}/{total} = {abstain_rate:.3f}")
    if errors:
        print(f"  Error rate:     {errors}/{total} = {error_rate:.3f}")

    summary = {
        "mode": mode,
        "total_questions": total,
        "eval_k": eval_k,
        "hit_at_k": hit_at_k,
        "mrr": mrr,
        "abstain_rate": abstain_rate,
        "hits": hits,
        "abstains": abstains,
        "errors": errors,
    }

    if mode == "retrieval":
        avg_latency_vec = (
            sum(r.latency_vec_ms for r in non_abstain) / len(non_abstain)
            if non_abstain else 0.0
        )
        avg_latency_fts = (
            sum(r.latency_fts_ms for r in non_abstain) / len(non_abstain)
            if non_abstain else 0.0
        )
        avg_latency_merge = (
            sum(r.latency_merge_ms for r in non_abstain) / len(non_abstain)
            if non_abstain else 0.0
        )
        avg_latency_total = (
            sum(r.latency_total_ms for r in results) / total
            if total > 0 else 0.0
        )
        print(f"  Avg latency:")
        print(f"    Vector:       {avg_latency_vec:.1f} ms")
        print(f"    FTS:          {avg_latency_fts:.1f} ms")
        print(f"    RRF merge:    {avg_latency_merge:.1f} ms")
        print(f"    Total:        {avg_latency_total:.1f} ms")

        summary.update({
            "avg_latency_vec_ms": round(avg_latency_vec, 1),
            "avg_latency_fts_ms": round(avg_latency_fts, 1),
            "avg_latency_merge_ms": round(avg_latency_merge, 1),
            "avg_latency_total_ms": round(avg_latency_total, 1),
        })
    else:
        # Full pipeline latency
        answered = [r for r in results if not r.error]
        avg_e2e = (
            sum(r.latency_e2e_ms for r in answered) / len(answered)
            if answered else 0.0
        )
        p50_e2e = sorted(r.latency_e2e_ms for r in answered)[len(answered) // 2] if answered else 0.0
        p95_idx = int(len(answered) * 0.95) if answered else 0
        p95_e2e = sorted(r.latency_e2e_ms for r in answered)[min(p95_idx, len(answered) - 1)] if answered else 0.0

        reranker_used_count = sum(1 for r in answered if r.reranker_used)
        avg_citations = (
            sum(r.num_citations for r in non_abstain) / len(non_abstain)
            if non_abstain else 0.0
        )

        print(f"  Avg citations:  {avg_citations:.1f}")
        print(f"  Reranker used:  {reranker_used_count}/{len(answered)}")
        print(f"  Avg latency (e2e):")
        print(f"    Mean:         {avg_e2e:.0f} ms")
        print(f"    P50:          {p50_e2e:.0f} ms")
        print(f"    P95:          {p95_e2e:.0f} ms")

        summary.update({
            "avg_citations": round(avg_citations, 1),
            "reranker_used_count": reranker_used_count,
            "avg_latency_e2e_ms": round(avg_e2e, 0),
            "p50_latency_e2e_ms": round(p50_e2e, 0),
            "p95_latency_e2e_ms": round(p95_e2e, 0),
        })

    print("=" * 70)
    return summary


def write_results_csv(results: list[EvalResult], output_path: str, mode: str) -> None:
    """Write per-question results to CSV."""
    output_file = Path(output_path)
    output_file.parent.mkdir(parents=True, exist_ok=True)

    if mode == "retrieval":
        headers = [
            "question", "expected_doc_title", "expected_abstain",
            "hit_at_k", "reciprocal_rank", "abstained",
            "top_doc_title", "num_vec_results", "num_fts_results", "num_rrf_results",
            "latency_vec_ms", "latency_fts_ms", "latency_merge_ms", "latency_total_ms",
        ]
        rows = [
            [
                r.question, r.expected_doc_title, r.expected_abstain,
                r.hit_at_k, f"{r.reciprocal_rank:.4f}", r.abstained,
                r.top_doc_title, r.num_vec_results, r.num_fts_results, r.num_rrf_results,
                f"{r.latency_vec_ms:.1f}", f"{r.latency_fts_ms:.1f}",
                f"{r.latency_merge_ms:.1f}", f"{r.latency_total_ms:.1f}",
            ]
            for r in results
        ]
    else:
        headers = [
            "question", "expected_doc_title", "expected_abstain",
            "hit_at_k", "reciprocal_rank", "abstained",
            "top_doc_title", "num_citations", "citation_doc_titles",
            "num_vec_results", "num_fts_results", "num_rrf_results",
            "reranker_used", "reranker_skipped",
            "latency_e2e_ms", "http_status", "error",
        ]
        rows = [
            [
                r.question, r.expected_doc_title, r.expected_abstain,
                r.hit_at_k, f"{r.reciprocal_rank:.4f}", r.abstained,
                r.top_doc_title, r.num_citations, r.citation_doc_titles,
                r.num_vec_results, r.num_fts_results, r.num_rrf_results,
                r.reranker_used, r.reranker_skipped,
                f"{r.latency_e2e_ms:.0f}", r.http_status, r.error,
            ]
            for r in results
        ]

    with open(output_file, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(headers)
        writer.writerows(rows)


def write_summary_json(summary: dict, output_path: str, config: dict) -> None:
    """Write summary metrics to JSON."""
    summary_path = Path(output_path).with_suffix(".summary.json")
    summary["config"] = config
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)
    logger.info("Summary written to %s", summary_path)


# ── Main entry points ────────────────────────────────────

def run_eval(
    questions_path: str,
    output_path: str,
    mode: str = "retrieval",
    database_url: str = DEFAULT_DATABASE_URL,
    tenant_id: str = DEFAULT_TENANT_ID,
    embedding_model: str = DEFAULT_EMBEDDING_MODEL,
    eval_k: int = DEFAULT_EVAL_K,
    k_vec: int = DEFAULT_K_VEC,
    k_fts: int = DEFAULT_K_FTS,
    rrf_k: int = DEFAULT_RRF_K,
    api_url: str = DEFAULT_API_URL,
) -> None:
    """Run evaluation in the specified mode."""
    # Load questions
    questions = load_questions(questions_path)
    if not questions:
        logger.error("No questions loaded from %s", questions_path)
        sys.exit(1)
    logger.info("Loaded %d evaluation questions (mode=%s)", len(questions), mode)

    # Run evaluation
    if mode == "retrieval":
        results = run_retrieval_eval(
            questions, database_url, tenant_id, embedding_model,
            eval_k, k_vec, k_fts, rrf_k,
        )
        config = {
            "mode": "retrieval",
            "k_vec": k_vec,
            "k_fts": k_fts,
            "rrf_k": rrf_k,
            "embedding_model": embedding_model,
            "tenant_id": tenant_id,
        }
    elif mode == "full":
        results = run_full_eval(questions, api_url, tenant_id, eval_k)
        config = {
            "mode": "full",
            "api_url": api_url,
            "eval_k": eval_k,
            "tenant_id": tenant_id,
        }
    else:
        logger.error("Unknown mode: %s (use 'retrieval' or 'full')", mode)
        sys.exit(1)

    # Compute and print metrics
    summary = compute_and_print_metrics(results, eval_k, mode)

    # Write outputs
    write_results_csv(results, output_path, mode)
    write_summary_json(summary, output_path, config)
    logger.info("Results written to %s", output_path)

    # Exit with non-zero if Hit@K is below threshold
    if summary["hit_at_k"] < 0.5:
        logger.warning(
            "Hit@%d = %.3f is below 0.5 threshold — needs improvement",
            eval_k, summary["hit_at_k"],
        )
        sys.exit(2)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="pro-rag evaluation harness (retrieval-only or full pipeline)",
    )
    parser.add_argument(
        "--mode", choices=["retrieval", "full"], default="retrieval",
        help="Evaluation mode: 'retrieval' (DB-only, no LLM) or 'full' (calls POST /v1/query). Default: retrieval",
    )
    parser.add_argument(
        "--questions", default="eval/questions.jsonl",
        help="Path to questions JSONL file (default: eval/questions.jsonl)",
    )
    parser.add_argument(
        "--output", default=None,
        help="Path to output CSV file (default: eval/results_<mode>.csv)",
    )
    parser.add_argument(
        "--k", type=int, default=DEFAULT_EVAL_K,
        help=f"Evaluation depth for Hit@K (default: {DEFAULT_EVAL_K})",
    )
    parser.add_argument(
        "--api-url", default=DEFAULT_API_URL,
        help=f"API base URL for full mode (default: {DEFAULT_API_URL})",
    )
    parser.add_argument(
        "--database-url", default=DEFAULT_DATABASE_URL,
        help="Database connection URL (retrieval mode only)",
    )
    parser.add_argument(
        "--tenant-id", default=DEFAULT_TENANT_ID,
        help="Tenant ID to evaluate",
    )
    parser.add_argument(
        "--embedding-model", default=DEFAULT_EMBEDDING_MODEL,
        help="Embedding model name (retrieval mode only)",
    )
    args = parser.parse_args()

    # Default output path based on mode
    output = args.output or f"eval/results_{args.mode}.csv"

    run_eval(
        questions_path=args.questions,
        output_path=output,
        mode=args.mode,
        database_url=args.database_url,
        tenant_id=args.tenant_id,
        embedding_model=args.embedding_model,
        eval_k=args.k,
        api_url=args.api_url,
    )


if __name__ == "__main__":
    main()
