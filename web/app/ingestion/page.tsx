"use client";

import { useEffect, useState, useCallback } from "react";
import { History, ChevronDown, ChevronRight } from "lucide-react";
import { listIngestionRuns } from "@/lib/api";
import type { IngestionRunItem } from "@/lib/types";
import { formatDate, formatDuration, shortId, cn } from "@/lib/utils";

export default function IngestionPage() {
  const [runs, setRuns] = useState<IngestionRunItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [expandedRun, setExpandedRun] = useState<string | null>(null);
  const limit = 20;

  const load = useCallback(async () => {
    try {
      const res = await listIngestionRuns(page, limit);
      setRuns(res.runs);
      setTotal(res.total);
    } catch (err) {
      console.error("Failed to load ingestion runs:", err);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    load();
  }, [load]);

  // Auto-refresh when any run is "running"
  useEffect(() => {
    const hasRunning = runs.some((r) => r.status === "running");
    if (!hasRunning) return;

    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, [runs, load]);

  const totalPages = Math.ceil(total / limit);

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Ingestion Runs</h1>
        {runs.some((r) => r.status === "running") && (
          <span className="flex items-center gap-2 text-xs text-blue-400">
            <span className="w-2 h-2 bg-blue-400 rounded-full animate-pulse" />
            Auto-refreshing...
          </span>
        )}
      </div>

      {loading ? (
        <div className="text-center py-12 text-text-dim">Loading...</div>
      ) : runs.length === 0 ? (
        <div className="text-center py-12">
          <History className="w-12 h-12 text-text-dim mx-auto mb-3" />
          <p className="text-text-dim">No ingestion runs yet.</p>
        </div>
      ) : (
        <div className="bg-surface border border-border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-text-dim text-left">
                <th className="px-4 py-3 font-medium w-8"></th>
                <th className="px-4 py-3 font-medium">Run ID</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Started</th>
                <th className="px-4 py-3 font-medium">Duration</th>
                <th className="px-4 py-3 font-medium">Docs</th>
                <th className="px-4 py-3 font-medium">Chunks</th>
                <th className="px-4 py-3 font-medium">Tokens</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <>
                  <tr
                    key={run.run_id}
                    className={cn(
                      "border-b border-border last:border-0 hover:bg-surface-2 transition-colors cursor-pointer",
                      run.status === "failed" && "bg-red-500/5"
                    )}
                    onClick={() =>
                      setExpandedRun(
                        expandedRun === run.run_id ? null : run.run_id
                      )
                    }
                  >
                    <td className="px-4 py-3">
                      {expandedRun === run.run_id ? (
                        <ChevronDown className="w-4 h-4 text-text-dim" />
                      ) : (
                        <ChevronRight className="w-4 h-4 text-text-dim" />
                      )}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {shortId(run.run_id)}…
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={run.status} />
                    </td>
                    <td className="px-4 py-3 text-text-dim">
                      {formatDate(run.started_at)}
                    </td>
                    <td className="px-4 py-3 text-text-dim">
                      {run.duration_ms != null
                        ? formatDuration(run.duration_ms)
                        : run.status === "running"
                        ? "..."
                        : "-"}
                    </td>
                    <td className="px-4 py-3">
                      {run.stats?.docs_processed ?? "-"}
                    </td>
                    <td className="px-4 py-3">
                      {run.stats?.chunks_created ?? "-"}
                    </td>
                    <td className="px-4 py-3">
                      {run.stats?.tokens_total?.toLocaleString() ?? "-"}
                    </td>
                  </tr>
                  {expandedRun === run.run_id && (
                    <tr key={`${run.run_id}-detail`}>
                      <td colSpan={8} className="px-4 py-3 bg-surface-2">
                        <div className="space-y-2 text-xs">
                          <div>
                            <span className="text-text-dim">Full Run ID: </span>
                            <span className="font-mono">{run.run_id}</span>
                          </div>
                          {run.finished_at && (
                            <div>
                              <span className="text-text-dim">Finished: </span>
                              <span>{formatDate(run.finished_at)}</span>
                            </div>
                          )}
                          {run.error && (
                            <div className="bg-red-500/10 border border-red-500/30 rounded p-2">
                              <span className="text-red-400 font-medium">
                                Error:{" "}
                              </span>
                              <span className="text-red-400">{run.error}</span>
                            </div>
                          )}
                          {run.config &&
                            Object.keys(run.config).length > 0 && (
                              <div>
                                <span className="text-text-dim">Config: </span>
                                <pre className="font-mono mt-1 text-foreground/70">
                                  {JSON.stringify(run.config, null, 2)}
                                </pre>
                              </div>
                            )}
                        </div>
                      </td>
                    </tr>
                  )}
                </>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-text-dim">
            {total} run{total !== 1 ? "s" : ""} total
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2"
            >
              Previous
            </button>
            <span className="px-3 py-1 text-text-dim">
              Page {page} of {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page === totalPages}
              className="px-3 py-1 bg-surface border border-border rounded text-sm disabled:opacity-40 hover:bg-surface-2"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const styles = {
    running: "bg-blue-500/20 text-blue-400",
    succeeded: "bg-green-500/20 text-green-400",
    failed: "bg-red-500/20 text-red-400",
  };
  return (
    <span
      className={cn(
        "px-2 py-0.5 rounded text-xs font-medium",
        styles[status as keyof typeof styles] || "bg-gray-500/20 text-gray-400"
      )}
    >
      {status}
    </span>
  );
}
