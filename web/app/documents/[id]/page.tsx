"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ChevronDown, ChevronRight, AlertTriangle } from "lucide-react";
import { getDocument, listChunks, deactivateDocument } from "@/lib/api";
import type { DocumentDetailResponse, ChunkListItem } from "@/lib/types";
import { formatDate, shortId, tokenColorClass, cn } from "@/lib/utils";

export default function DocumentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const docId = params.id as string;

  const [doc, setDoc] = useState<DocumentDetailResponse | null>(null);
  const [chunks, setChunks] = useState<ChunkListItem[]>([]);
  const [chunkTotal, setChunkTotal] = useState(0);
  const [chunkPage, setChunkPage] = useState(1);
  const [expandedChunk, setExpandedChunk] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [showDeactivateConfirm, setShowDeactivateConfirm] = useState(false);
  const [deactivating, setDeactivating] = useState(false);
  const chunkLimit = 50;

  const loadDoc = useCallback(async () => {
    try {
      const d = await getDocument(docId);
      setDoc(d);
    } catch (err) {
      console.error("Failed to load document:", err);
    }
  }, [docId]);

  const loadChunks = useCallback(async () => {
    try {
      const res = await listChunks(docId, chunkPage, chunkLimit);
      setChunks(res.chunks);
      setChunkTotal(res.total);
    } catch (err) {
      console.error("Failed to load chunks:", err);
    }
  }, [docId, chunkPage]);

  useEffect(() => {
    async function init() {
      setLoading(true);
      await Promise.all([loadDoc(), loadChunks()]);
      setLoading(false);
    }
    init();
  }, [loadDoc, loadChunks]);

  const handleDeactivate = async () => {
    setDeactivating(true);
    try {
      await deactivateDocument(docId);
      await loadDoc();
      setShowDeactivateConfirm(false);
    } catch (err) {
      console.error("Failed to deactivate:", err);
    } finally {
      setDeactivating(false);
    }
  };

  if (loading) {
    return (
      <div className="p-8 flex items-center justify-center h-full">
        <div className="text-text-dim">Loading document...</div>
      </div>
    );
  }

  if (!doc) {
    return (
      <div className="p-8 text-center">
        <p className="text-text-dim">Document not found.</p>
        <Link href="/documents" className="text-accent text-sm hover:underline mt-2 inline-block">
          Back to documents
        </Link>
      </div>
    );
  }

  const activeVersion = doc.versions.find((v) => v.is_active);
  const chunkTotalPages = Math.ceil(chunkTotal / chunkLimit);

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link href="/documents" className="text-text-dim hover:text-foreground">
          <ArrowLeft className="w-5 h-5" />
        </Link>
        <div className="flex-1">
          <h1 className="text-2xl font-bold">{doc.title}</h1>
          <div className="flex items-center gap-3 mt-1 text-sm text-text-dim">
            <span className="px-2 py-0.5 bg-surface-2 rounded text-xs font-mono uppercase">
              {doc.source_type}
            </span>
            <span>Created {formatDate(doc.created_at)}</span>
          </div>
        </div>
        {activeVersion && (
          <button
            onClick={() => setShowDeactivateConfirm(true)}
            className="px-3 py-1.5 bg-red-500/10 border border-red-500/30 text-red-400 rounded-lg text-sm hover:bg-red-500/20 transition-colors"
          >
            Deactivate
          </button>
        )}
      </div>

      {/* Deactivate confirmation */}
      {showDeactivateConfirm && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4 flex items-start gap-3">
          <AlertTriangle className="w-5 h-5 text-red-400 shrink-0 mt-0.5" />
          <div className="flex-1">
            <p className="text-sm text-red-400 font-medium">
              Deactivate this document?
            </p>
            <p className="text-xs text-text-dim mt-1">
              The document will no longer appear in query results. Data is not deleted.
            </p>
            <div className="flex gap-2 mt-3">
              <button
                onClick={handleDeactivate}
                disabled={deactivating}
                className="px-3 py-1 bg-red-500 text-white rounded text-sm hover:bg-red-600 disabled:opacity-50"
              >
                {deactivating ? "Deactivating..." : "Confirm"}
              </button>
              <button
                onClick={() => setShowDeactivateConfirm(false)}
                className="px-3 py-1 bg-surface border border-border rounded text-sm hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Document metadata */}
      <div className="bg-surface border border-border rounded-lg p-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
        <div>
          <div className="text-text-dim text-xs mb-1">Source URI</div>
          <div className="font-mono text-xs truncate" title={doc.source_uri}>
            {doc.source_uri}
          </div>
        </div>
        <div>
          <div className="text-text-dim text-xs mb-1">Content Hash</div>
          <div className="font-mono text-xs">{shortId(doc.content_hash)}…</div>
        </div>
        <div>
          <div className="text-text-dim text-xs mb-1">Active Version</div>
          <div>{activeVersion ? activeVersion.version_label : "None"}</div>
        </div>
        <div>
          <div className="text-text-dim text-xs mb-1">Chunks / Tokens</div>
          <div>
            {activeVersion
              ? `${activeVersion.chunk_count} / ${activeVersion.total_tokens.toLocaleString()}`
              : "-"}
          </div>
        </div>
      </div>

      {/* Version history */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Version History</h2>
        <div className="bg-surface border border-border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-text-dim text-left">
                <th className="px-4 py-2 font-medium">Version</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Effective</th>
                <th className="px-4 py-2 font-medium">Chunks</th>
                <th className="px-4 py-2 font-medium">Tokens</th>
              </tr>
            </thead>
            <tbody>
              {doc.versions.map((v) => (
                <tr
                  key={v.doc_version_id}
                  className={cn(
                    "border-b border-border last:border-0",
                    v.is_active && "bg-accent/5"
                  )}
                >
                  <td className="px-4 py-2 font-mono">{v.version_label}</td>
                  <td className="px-4 py-2">
                    {v.is_active ? (
                      <span className="px-2 py-0.5 bg-green-500/20 text-green-400 rounded text-xs">
                        active
                      </span>
                    ) : (
                      <span className="px-2 py-0.5 bg-gray-500/20 text-gray-400 rounded text-xs">
                        inactive
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2 text-text-dim">
                    {formatDate(v.effective_at)}
                  </td>
                  <td className="px-4 py-2">{v.chunk_count}</td>
                  <td className="px-4 py-2">{v.total_tokens.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Chunk browser */}
      <div>
        <h2 className="text-lg font-semibold mb-3">
          Chunks{" "}
          <span className="text-text-dim text-sm font-normal">
            ({chunkTotal} total)
          </span>
        </h2>
        {chunks.length === 0 ? (
          <p className="text-text-dim text-sm">No chunks found.</p>
        ) : (
          <div className="space-y-2">
            {chunks.map((chunk) => (
              <div
                key={chunk.chunk_id}
                className="bg-surface border border-border rounded-lg overflow-hidden"
              >
                <button
                  onClick={() =>
                    setExpandedChunk(
                      expandedChunk === chunk.chunk_id ? null : chunk.chunk_id
                    )
                  }
                  className="w-full px-4 py-3 flex items-center gap-3 text-left hover:bg-surface-2 transition-colors"
                >
                  {expandedChunk === chunk.chunk_id ? (
                    <ChevronDown className="w-4 h-4 text-text-dim shrink-0" />
                  ) : (
                    <ChevronRight className="w-4 h-4 text-text-dim shrink-0" />
                  )}
                  <span className="text-text-dim text-xs font-mono w-8">
                    #{chunk.ordinal}
                  </span>
                  <span className="flex-1 text-sm truncate">
                    {chunk.heading_path
                      ? (Array.isArray(chunk.heading_path)
                          ? chunk.heading_path
                          : []
                        ).join(" › ") || chunk.text.substring(0, 80)
                      : chunk.text.substring(0, 80)}
                  </span>
                  <span
                    className={cn(
                      "px-2 py-0.5 rounded text-xs font-mono shrink-0",
                      tokenColorClass(chunk.token_count)
                    )}
                  >
                    {chunk.token_count} tok
                  </span>
                  <span className="px-2 py-0.5 bg-surface-2 rounded text-xs font-mono shrink-0">
                    {chunk.chunk_type}
                  </span>
                </button>
                {expandedChunk === chunk.chunk_id && (
                  <div className="px-4 pb-4 border-t border-border">
                    <pre className="mt-3 text-sm whitespace-pre-wrap text-foreground/80 leading-relaxed max-h-96 overflow-y-auto">
                      {chunk.text}
                    </pre>
                    <div className="mt-2 text-xs text-text-dim font-mono">
                      ID: {chunk.chunk_id}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Chunk pagination */}
        {chunkTotalPages > 1 && (
          <div className="flex items-center justify-between text-sm mt-4">
            <span className="text-text-dim">
              {chunkTotal} chunk{chunkTotal !== 1 ? "s" : ""}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setChunkPage((p) => Math.max(1, p - 1))}
                disabled={chunkPage === 1}
                className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2"
              >
                Previous
              </button>
              <span className="px-3 py-1 text-text-dim">
                Page {chunkPage} of {chunkTotalPages}
              </span>
              <button
                onClick={() =>
                  setChunkPage((p) => Math.min(chunkTotalPages, p + 1))
                }
                disabled={chunkPage === chunkTotalPages}
                className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
