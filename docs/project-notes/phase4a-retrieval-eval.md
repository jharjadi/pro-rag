# Phase 4a: Retrieval-Only Eval Results

> Date: 2026-02-14 | Spec: §11 | Plan: Phase 4a

## Summary

Retrieval quality is **excellent** on the 5-doc test corpus. No chunking or retrieval parameter changes needed at this stage. Phase 4a gate is passed — proceed to Phase 3b and Phase 5.

## Corpus

| Document | Chunks | Tokens |
|----------|--------|--------|
| Acme Corp IT Security Policy | 14 | 1,477 |
| Acme Corp Employee Onboarding Guide | 14 | 1,669 |
| Acme Corp Expense Reimbursement Policy | 14 | 1,346 |
| Acme Corp Remote Work Policy | 13 | 1,285 |
| Acme Corp Incident Response Procedure | 18 | 1,935 |
| **Total** | **73** | **7,712** |

## Eval Results

| Metric | Value |
|--------|-------|
| Questions | 40 |
| Hit@10 | 40/40 = **1.000** |
| MRR | **0.975** |
| Abstain rate | 0/40 = 0.000 |
| Avg vector latency | 2.8 ms |
| Avg FTS latency | 0.9 ms |
| Avg RRF merge latency | 0.0 ms |
| Avg total latency | 3.7 ms |

## Configuration

- Embedding model: BAAI/bge-base-en-v1.5 (768-dim)
- K_VEC: 50, K_FTS: 50, RRF_K: 60
- Chunk target: 450 tokens, hard cap: 800 tokens
- Eval depth: Hit@10

## Analysis

### Strengths
- **Perfect Hit@10**: Every question found the correct document within top 10 results.
- **Near-perfect MRR (0.975)**: 39/40 questions had the correct document at rank 1.
- **Fast retrieval**: Sub-4ms total latency for hybrid vector+FTS+RRF pipeline.
- **Table-aware chunking working**: Questions about table data (classification levels, expense limits, severity levels) all retrieved correctly.

### One Imperfect Result
- Q20: "What is the annual professional development expense limit?" → Expected: Expense Reimbursement Policy, Got rank 1: Employee Onboarding Guide (RR=0.50).
- **Root cause**: Both documents mention a $3,000 annual learning/development budget. The Onboarding Guide mentions it in the Training section, and the Expense Policy lists it in the expense categories table. This is a legitimate ambiguity — both documents contain the answer.
- **Action**: No change needed. This is expected behavior for overlapping content across documents.

### Observations
- RRF merge is effectively instant (0.0 ms) at this scale (73 chunks).
- Vector search dominates latency (2.8 ms avg) vs FTS (0.9 ms avg).
- No abstains on any question — good for this corpus where all questions have answers.
- Chunk sizes are well within spec limits (avg ~106 tokens/chunk, well under 800 hard cap).

## Decision

**Phase 4a gate: PASSED.** Retrieval quality exceeds the 0.5 Hit@K threshold by a wide margin. No parameter tuning needed. Proceed to:
- Phase 3b (expand ingestion formats) — can start when needed
- Phase 5 (Go query API) — can start immediately
