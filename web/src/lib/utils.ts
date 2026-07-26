import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatMs(ms?: number | null) {
  if (ms == null || ms <= 0) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatTime(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("zh-CN", { hour12: false });
  } catch {
    return iso;
  }
}

export function formatBytes(bytes?: number | null) {
  if (bytes == null || bytes < 1) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GiB`;
}

export function formatDuration(seconds?: number | null) {
  if (seconds == null || seconds < 0) return "—";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days) return `${days}天 ${hours}小时`;
  if (hours) return `${hours}小时 ${minutes}分`;
  return `${minutes}分`;
}

export function formatPercent(value?: number | null, digits = 0) {
  if (value == null || Number.isNaN(value)) return "—";
  return `${(value * 100).toFixed(digits)}%`;
}

export function gradeColor(grade?: string) {
  switch (grade) {
    case "S":
      return "text-accent border-accent/45 bg-accent/10";
    case "A":
      return "text-primary border-primary/45 bg-primary/10";
    case "B":
      return "text-success border-success/45 bg-success/10";
    case "C":
      return "text-sky-300 border-sky-400/40 bg-sky-400/10";
    case "D":
      return "text-orange-300 border-orange-400/40 bg-orange-400/10";
    default:
      return "text-muted-foreground border-border bg-muted";
  }
}

/** Badge variant matching a 0-100 health score, shared by sources and alerts. */
export function healthVariant(score: number): "success" | "warn" | "danger" {
  if (score >= 80) return "success";
  if (score >= 50) return "warn";
  return "danger";
}
