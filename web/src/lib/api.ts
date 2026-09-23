// API 层：REST + 单条复用 WebSocket。
import { b64encode } from "./utils";

const TOKEN_KEY = "termirai_token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || "";
}
export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t);
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export async function api(path: string, opts: RequestInit = {}): Promise<any> {
  const resp = await fetch("/api/v1" + path, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer " + getToken(),
      ...(opts.headers || {}),
    },
  });
  if (resp.status === 401) {
    clearToken();
    location.hash = "#/login";
    throw Object.assign(new Error("未认证"), { code: "unauthorized" });
  }
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw Object.assign(new Error(data.error || `HTTP ${resp.status}`), { code: data.code, params: data.params });
  return data;
}

export const rest = {
  captcha: () => fetch("/api/v1/captcha").then((r) => r.json()),
  login: (body: any) =>
    fetch("/api/v1/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(async (r) => {
      const d = await r.json();
      if (!r.ok) {
        const e: any = Object.assign(new Error(d.error || "登录失败"), { code: d.code, params: d.params });
        e.locked_seconds = d.locked_seconds;
        throw e;
      }
      return d;
    }),
  clients: () => api("/clients").then((d) => d.clients),
  deleteClient: (id: string) => api(`/clients/${id}`, { method: "DELETE" }),
  fileList: (id: string, path: string) =>
    api(`/clients/${id}/files?path=${encodeURIComponent(path)}`).then((d) => d.entries || []),
  fileRead: (id: string, path: string) =>
    api(`/clients/${id}/file?path=${encodeURIComponent(path)}`).then((d) => d.data as string),
  fileWrite: (id: string, path: string, content: string) =>
    api(`/clients/${id}/file`, {
      method: "POST",
      body: JSON.stringify({ path, data: b64encode(content) }),
    }),
  fileDelete: (id: string, path: string) =>
    api(`/clients/${id}/file?path=${encodeURIComponent(path)}`, { method: "DELETE" }),
  fileRename: (id: string, path: string, newPath: string) =>
    api(`/clients/${id}/file/rename`, { method: "POST", body: JSON.stringify({ path, new_path: newPath }) }),
  fileMkdir: (id: string, path: string) =>
    api(`/clients/${id}/file/mkdir`, { method: "POST", body: JSON.stringify({ path }) }),
  aiConfig: () => api("/ai/config"),
  saveAIConfig: (body: { base_url: string; api_key: string; model: string; default_mode: string }) =>
    api("/ai/config", { method: "POST", body: JSON.stringify(body) }),
  testAI: () => api("/ai/test", { method: "POST" }),
};

// ---- 复用 WebSocket ----

type Handler = (msg: any) => void;

class WSClient {
  private ws: WebSocket | null = null;
  private handlers = new Set<Handler>();
  private reconnectTimer: any = null;
  connected = false;
  onConnect: (() => void) | null = null;

  connect() {
    if (this.ws) return;
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws?token=${encodeURIComponent(getToken())}`);
    this.ws = ws;
    ws.onopen = () => {
      this.connected = true;
      this.onConnect?.();
    };
    ws.onmessage = (ev) => {
      let msg: any;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return;
      }
      this.handlers.forEach((h) => h(msg));
    };
    ws.onclose = () => {
      this.ws = null;
      this.connected = false;
      this.handlers.forEach((h) => h({ type: "_ws_closed" }));
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = setTimeout(() => this.connect(), 2000);
    };
    ws.onerror = () => ws.close();
  }

  close() {
    clearTimeout(this.reconnectTimer);
    this.ws?.close();
    this.ws = null;
  }

  send(msg: any) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  subscribe(h: Handler): () => void {
    this.handlers.add(h);
    return () => this.handlers.delete(h);
  }
}

export const ws = new WSClient();
