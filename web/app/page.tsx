"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { FileText, MessageSquare, Upload, Activity, Database, Hash } from "lucide-react";
import { listDocuments, listIngestionRuns, checkHealth } from "@/lib/api";
import type { DocumentListItem, IngestionRunItem } from "@/lib/types";
import { formatDate, cn } from "@/lib/utils";

export default function DashboardPage() {
  const [docCount, setDocCount] = useState<number>(0);
  const [chunkCount, setChunkCount] = useState<number>(0);
  const [totalTokens, setTotalTokens] = useState<number>(0);
  const [recentRuns, setRecentRuns] = useState<IngestionRunItem[]>([]);
  const [healthy, setHealthy] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [docs, runs, health] = await Promise.all([
          listDocuments(1, 100),
          listIngestionRuns(1, 5),
          checkHealth().catch(() => ({ status: "unhealthy" })),
        ]);

        setDocCount(docs.total);
        let chunks = 0;
        let tokens = 0;
        for (const d of docs.documents) {
          if (d.active_version) {
            chunks += d.active_version.chunk_count;
            tokens += d.active_version.total_tokens;
          }
        }
        setChunkCount(chunks);
        setTotalTokens(tokens);
        setRecentRuns(runs.runs);
        setHealthy(health.status === "ok");
      } catch {
        setHealthy(false);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  if (loading) {
    return (
      <div className="p-8 flex items-center justify-center h-full">
        <div className="text-text-dim">Loading dashboard...</div>
      </div>
    );
  }

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-8">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <div className="flex items-center gap-2">
          <div
            className={cn(
              "w-2 h-2 rounded-full",
              healthy ? "bg-green-400" : "bg-red-400"
            )}
          />
          <span className="text-xs text-text-dim">
            {healthy ? "System healthy" : "System unhealthy"}
          </span>
        </div>
      </div>

      {/* Stats cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <StatCard
          icon={<FileText className="w-5 h-5" />}
          label="Documents"
          value={docCount}
        />
        <StatCard
          icon={<Database className="w-5 h-5" />}
          label="Chunks"
          value={chunkCount}
        />
        <StatCard
          icon={<Hash className="w-5 h-5" />}
          label="Total Tokens"
          value={totalTokens.toLocaleString()}
        />
      </div>

      {/* Quick actions */}
      <div className="flex gap-3">
        <Link
          href="/documents/new"
          className="flex items-center gap-2 px-4 py-2 bg-accent text-background rounded-lg text-sm font-medium hover:bg-accent/90 transition-colors"
        >
          <Upload className="w-4 h-4" />
          Upload Document
        </Link>
        <Link
          href="/chat"
          className="flex items-center gap-2 px-4 py-2 bg-surface-2 border border-border text-foreground rounded-lg text-sm font-medium hover:bg-surface transition-colors"
        >
          <MessageSquare className="w-4 h-4" />
          Ask Question
        </Link>
      </div>

      {/* Recent ingestion runs */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Recent Ingestion Runs</h2>
        {recentRuns.length === 0 ? (
          <p className="text-text-dim text-sm">No ingestion runs yet.</p>
        ) : (
          <div className="bg-surface border border-border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-text-dim text-left">
                  <th className="px-4 py-2 font-medium">Status</th>
                  <th className="px-4 py-2 font-medium">Started</th>
                  <th className="px-4 py-2 font-medium">Chunks</th>
                  <th className="px-4 py-2 font-medium">Tokens</th>
                </tr>
              </thead>
              <tbody>
                {recentRuns.map((run) => (
                  <tr key={run.run_id} className="border-b border-border last:border-0">
                    <td className="px-4 py-2">
                      <StatusBadge status={run.status} />
                    </td>
                    <td className="px-4 py-2 text-text-dim">
                      {formatDate(run.started_at)}
                    </td>
                    <td className="px-4 py-2">
                      {run.stats?.chunks_created ?? "-"}
                    </td>
                    <td className="px-4 py-2">
                      {run.stats?.tokens_total?.toLocaleString() ?? "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

function StatCard({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: number | string;
}) {
  return (
    <div className="bg-surface border border-border rounded-lg p-4 flex items-center gap-4">
      <div className="text-accent">{icon}</div>
      <div>
        <div className="text-2xl font-bold">{value}</div>
        <div className="text-xs text-text-dim">{label}</div>
      </div>
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
