"use client";

import { useState, useRef, useCallback } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Upload, FileText, CheckCircle, XCircle, Loader2 } from "lucide-react";
import { uploadDocument, getIngestionRun } from "@/lib/api";
import { cn } from "@/lib/utils";

const SUPPORTED_FORMATS = [".docx", ".pdf", ".html", ".htm"];
const MAX_SIZE_MB = 50;

export default function UploadPage() {
  const router = useRouter();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [dragOver, setDragOver] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [status, setStatus] = useState<
    "idle" | "uploading" | "processing" | "succeeded" | "failed"
  >("idle");
  const [runId, setRunId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleFile = (f: File) => {
    const ext = "." + f.name.split(".").pop()?.toLowerCase();
    if (!SUPPORTED_FORMATS.includes(ext)) {
      setError(`Unsupported format: ${ext}. Supported: ${SUPPORTED_FORMATS.join(", ")}`);
      return;
    }
    if (f.size > MAX_SIZE_MB * 1024 * 1024) {
      setError(`File too large: ${(f.size / 1024 / 1024).toFixed(1)}MB. Max: ${MAX_SIZE_MB}MB`);
      return;
    }
    setFile(f);
    setError(null);
    // Auto-populate title from filename
    if (!title) {
      const name = f.name.replace(/\.[^.]+$/, "").replace(/[_-]/g, " ");
      setTitle(name.charAt(0).toUpperCase() + name.slice(1));
    }
  };

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      const f = e.dataTransfer.files[0];
      if (f) handleFile(f);
    },
    [title]
  );

  const pollRunStatus = async (id: string) => {
    const maxAttempts = 120; // 10 minutes at 5s intervals
    for (let i = 0; i < maxAttempts; i++) {
      await new Promise((r) => setTimeout(r, 5000));
      try {
        const run = await getIngestionRun(id);
        if (run.status === "succeeded") {
          setStatus("succeeded");
          return;
        }
        if (run.status === "failed") {
          setStatus("failed");
          setError(run.error || "Ingestion failed");
          return;
        }
      } catch {
        // Continue polling
      }
    }
    setStatus("failed");
    setError("Ingestion timed out");
  };

  const handleSubmit = async () => {
    if (!file || !title) return;

    setUploading(true);
    setStatus("uploading");
    setError(null);

    try {
      const res = await uploadDocument(file, title);
      setRunId(res.run_id);
      setStatus("processing");

      // Poll for completion
      await pollRunStatus(res.run_id);
    } catch (err: unknown) {
      setStatus("failed");
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="p-8 max-w-2xl mx-auto space-y-6">
      <h1 className="text-2xl font-bold">Upload Document</h1>

      {/* Drop zone */}
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        className={cn(
          "border-2 border-dashed rounded-lg p-12 text-center cursor-pointer transition-colors",
          dragOver
            ? "border-accent bg-accent/5"
            : file
            ? "border-green-500/50 bg-green-500/5"
            : "border-border hover:border-accent/50"
        )}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept={SUPPORTED_FORMATS.join(",")}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) handleFile(f);
          }}
          className="hidden"
        />
        {file ? (
          <div className="flex flex-col items-center gap-2">
            <FileText className="w-10 h-10 text-green-400" />
            <p className="text-sm font-medium">{file.name}</p>
            <p className="text-xs text-text-dim">
              {(file.size / 1024).toFixed(0)} KB
            </p>
            <p className="text-xs text-accent">Click to change file</p>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-2">
            <Upload className="w-10 h-10 text-text-dim" />
            <p className="text-sm text-text-dim">
              Drag and drop a file here, or click to browse
            </p>
            <p className="text-xs text-text-dim">
              Supported: {SUPPORTED_FORMATS.join(", ")} (max {MAX_SIZE_MB}MB)
            </p>
          </div>
        )}
      </div>

      {/* Title input */}
      <div>
        <label className="block text-sm font-medium mb-1">Document Title</label>
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="e.g., Acme Corp IT Security Policy"
          className="w-full px-4 py-2 bg-surface border border-border rounded-lg text-sm text-foreground placeholder:text-text-dim focus:outline-none focus:border-accent"
        />
      </div>

      {/* Error */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 flex items-start gap-2">
          <XCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {/* Status */}
      {status === "processing" && (
        <div className="bg-blue-500/10 border border-blue-500/30 rounded-lg p-3 flex items-center gap-2">
          <Loader2 className="w-4 h-4 text-blue-400 animate-spin" />
          <p className="text-sm text-blue-400">
            Processing document... This may take a minute.
          </p>
        </div>
      )}

      {status === "succeeded" && (
        <div className="bg-green-500/10 border border-green-500/30 rounded-lg p-3 flex items-center gap-2">
          <CheckCircle className="w-4 h-4 text-green-400" />
          <div>
            <p className="text-sm text-green-400">Document ingested successfully!</p>
            <Link
              href="/documents"
              className="text-xs text-accent hover:underline mt-1 inline-block"
            >
              View in document list →
            </Link>
          </div>
        </div>
      )}

      {/* Submit button */}
      <button
        onClick={handleSubmit}
        disabled={!file || !title || uploading || status === "succeeded"}
        className="w-full px-4 py-2.5 bg-accent text-background rounded-lg text-sm font-medium hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors flex items-center justify-center gap-2"
      >
        {uploading ? (
          <>
            <Loader2 className="w-4 h-4 animate-spin" />
            {status === "uploading" ? "Uploading..." : "Processing..."}
          </>
        ) : (
          <>
            <Upload className="w-4 h-4" />
            Upload & Ingest
          </>
        )}
      </button>
    </div>
  );
}
