"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { Search, FileText, Plus } from "lucide-react";
import { listDocuments } from "@/lib/api";
import type { DocumentListItem } from "@/lib/types";
import { formatDate, cn } from "@/lib/utils";

export default function DocumentsPage() {
  const [documents, setDocuments] = useState<DocumentListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const limit = 20;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listDocuments(page, limit, search);
      setDocuments(res.documents);
      setTotal(res.total);
    } catch (err) {
      console.error("Failed to load documents:", err);
    } finally {
      setLoading(false);
    }
  }, [page, search]);

  useEffect(() => {
    load();
  }, [load]);

  const totalPages = Math.ceil(total / limit);

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Documents</h1>
        <Link
          href="/documents/new"
          className="flex items-center gap-2 px-4 py-2 bg-accent text-background rounded-lg text-sm font-medium hover:bg-accent/90 transition-colors"
        >
          <Plus className="w-4 h-4" />
          Upload New
        </Link>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-dim" />
        <input
          type="text"
          placeholder="Search documents..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(1);
          }}
          className="w-full pl-10 pr-4 py-2 bg-surface border border-border rounded-lg text-sm text-foreground placeholder:text-text-dim focus:outline-none focus:border-accent"
        />
      </div>

      {/* Table */}
      {loading ? (
        <div className="text-center py-12 text-text-dim">Loading...</div>
      ) : documents.length === 0 ? (
        <div className="text-center py-12">
          <FileText className="w-12 h-12 text-text-dim mx-auto mb-3" />
          <p className="text-text-dim">No documents found.</p>
          <Link
            href="/documents/new"
            className="text-accent text-sm hover:underline mt-2 inline-block"
          >
            Upload your first document
          </Link>
        </div>
      ) : (
        <div className="bg-surface border border-border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-text-dim text-left">
                <th className="px-4 py-3 font-medium">Title</th>
                <th className="px-4 py-3 font-medium">Type</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Chunks</th>
                <th className="px-4 py-3 font-medium">Created</th>
              </tr>
            </thead>
            <tbody>
              {documents.map((doc) => (
                <tr
                  key={doc.doc_id}
                  className="border-b border-border last:border-0 hover:bg-surface-2 transition-colors"
                >
                  <td className="px-4 py-3">
                    <Link
                      href={`/documents/${doc.doc_id}`}
                      className="text-accent hover:underline font-medium"
                    >
                      {doc.title}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <span className="px-2 py-0.5 bg-surface-2 rounded text-xs font-mono uppercase">
                      {doc.source_type}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    {doc.active_version ? (
                      <span className="px-2 py-0.5 bg-green-500/20 text-green-400 rounded text-xs font-medium">
                        active
                      </span>
                    ) : (
                      <span className="px-2 py-0.5 bg-gray-500/20 text-gray-400 rounded text-xs font-medium">
                        inactive
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-text-dim">
                    {doc.active_version?.chunk_count ?? "-"}
                  </td>
                  <td className="px-4 py-3 text-text-dim">
                    {formatDate(doc.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-text-dim">
            {total} document{total !== 1 ? "s" : ""} total
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2 transition-colors"
            >
              Previous
            </button>
            <span className="px-3 py-1 text-text-dim">
              Page {page} of {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page === totalPages}
              className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2 transition-colors"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
