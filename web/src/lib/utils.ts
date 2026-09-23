import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import type { ClientInfo } from "./types";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// 客户端展示名：别名优先
export function clientName(c?: ClientInfo | null): string {
  return c ? c.alias || c.name : "";
}

// 客户端 + 终端的展示标签，如 "web1:部署终端"
export function termLabel(c: ClientInfo | undefined, clientID: string, termID?: string): string {
  const cn = c ? clientName(c) : clientID.slice(0, 8);
  if (!termID) return cn;
  const t = c?.terminals?.[termID];
  return `${cn}:${t?.title || termID.slice(0, 8)}`;
}

export function b64encode(s: string): string {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)));
}

export function b64decode(s: string): string {
  return new TextDecoder().decode(Uint8Array.from(atob(s), (c) => c.charCodeAt(0)));
}

export function formatTime(ts: number, lang: "zh" | "en" = "zh"): string {
  return new Date(ts * 1000).toLocaleString(lang === "zh" ? "zh-CN" : "en-US", { hour12: false });
}
