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

export function gradeColor(grade?: string) {
  switch (grade) {
    case "S":
      return "text-amber-300 border-amber-400/40 bg-amber-400/10";
    case "A":
      return "text-cyan-300 border-cyan-400/40 bg-cyan-400/10";
    case "B":
      return "text-emerald-300 border-emerald-400/40 bg-emerald-400/10";
    case "C":
      return "text-sky-300 border-sky-400/40 bg-sky-400/10";
    case "D":
      return "text-orange-300 border-orange-400/40 bg-orange-400/10";
    default:
      return "text-zinc-400 border-zinc-500/40 bg-zinc-500/10";
  }
}
