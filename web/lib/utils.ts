import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-AU", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.floor(seconds % 60);
  return `${minutes}m ${remainingSeconds}s`;
}

export function shortId(id: string): string {
  return id.substring(0, 8);
}

export function tokenColorClass(tokenCount: number): string {
  if (tokenCount < 400) return "bg-green-500/20 text-green-400";
  if (tokenCount < 600) return "bg-yellow-500/20 text-yellow-400";
  if (tokenCount < 800) return "bg-orange-500/20 text-orange-400";
  return "bg-red-500/20 text-red-400";
}
