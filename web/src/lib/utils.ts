import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatMs(value?: number | null) {
  if (value == null || value <= 0) return "—";
  if (value < 1000) return Math.round(value) + " ms";
  return (value / 1000).toFixed(1) + " s";
}

export function formatTime(value?: string | null) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

export function formatBytes(value?: number | null) {
  if (value == null || value < 1) return "—";
  if (value < 1024) return value + " B";
  if (value < 1024 * 1024) return (value / 1024).toFixed(1) + " KiB";
  if (value < 1024 * 1024 * 1024) return (value / 1024 / 1024).toFixed(1) + " MiB";
  return (value / 1024 / 1024 / 1024).toFixed(1) + " GiB";
}

export function formatDuration(value?: number | null) {
  if (value == null || value < 0) return "—";
  const days = Math.floor(value / 86400);
  const hours = Math.floor((value % 86400) / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  if (days) return days + " 天 " + hours + " 小时";
  if (hours) return hours + " 小时 " + minutes + " 分";
  return minutes + " 分";
}

export function formatPercent(value?: number | null, digits = 0) {
  if (value == null || Number.isNaN(value)) return "—";
  return (value * 100).toFixed(digits) + "%";
}

export function formatNumber(value?: number | null, digits = 1) {
  if (value == null || Number.isNaN(value)) return "—";
  return value.toLocaleString("zh-CN", { maximumFractionDigits: digits });
}
