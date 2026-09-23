// 全局状态（zustand）：客户端列表、终端标签、AI 会话、页面状态持久化。
import { create } from "zustand";
import { ws } from "./api";
import type { AISession, Approval, ClientInfo, TermTab, UIState } from "./types";

interface Store {
  connected: boolean;
  clients: ClientInfo[];
  aiSessions: AISession[];
  approvals: Approval[];
  aiDefaultMode: string;
  aiEnabled: boolean;

  // 页面状态（持久化到服务端）
  page: string;
  termTabs: TermTab[];
  activeTab: string;
  filesState: { client_id: string; path: string; open_file?: string } | null;
  aiSessionID: string;

  // 跨页动作：客户端页点"终端"后，由终端页消费（不持久化）
  // term_id 非空=接入已有终端；否则新建（shell 为空=客户端默认）
  pendingTerm: { client_id: string; term_id?: string; title?: string; shell?: string } | null;

  hydrated: boolean;

  set: (p: Partial<Store>) => void;
  persistUI: () => void;
}

let persistTimer: any = null;

export const useStore = create<Store>((set, get) => ({
  connected: false,
  clients: [],
  aiSessions: [],
  approvals: [],
  aiDefaultMode: "approval",
  aiEnabled: false,
  page: "clients",
  termTabs: [],
  activeTab: "",
  filesState: null,
  aiSessionID: "",
  pendingTerm: null,
  hydrated: false,
  set: (p) => set(p),
  persistUI: () => {
    clearTimeout(persistTimer);
    persistTimer = setTimeout(() => {
      const s = get();
      const ui: UIState = {
        page: s.page,
        term_tabs: s.termTabs,
        active_tab: s.activeTab,
        files: s.filesState || undefined,
        ai_session_id: s.aiSessionID,
      };
      ws.send({ type: "ui_state", state: ui });
    }, 500);
  },
}));

// ---- WS 消息入口 ----

export function initWSHandlers() {
  ws.onConnect = () => {
    useStore.setState({ connected: true });
    // 重连后重新订阅所有打开的终端
    const { termTabs } = useStore.getState();
    for (const t of termTabs) {
      if (!t.closed) ws.send({ type: "term_sub", client_id: t.client_id, term_id: t.term_id });
    }
  };
  ws.subscribe((msg) => {
    const s = useStore.getState();
    switch (msg.type) {
      case "_ws_closed":
        useStore.setState({ connected: false });
        break;
      case "state": {
        // 服务端全量状态：首次水合页面状态
        const ui: UIState = msg.ui_state || {};
        useStore.setState({
          clients: msg.clients || [],
          aiSessions: msg.ai_sessions || [],
          aiDefaultMode: msg.ai_default_mode || "approval",
          aiEnabled: !!msg.ai_enabled,
          page: ui.page || "clients",
          termTabs: (ui.term_tabs || []).filter((t) => !t.closed),
          activeTab: ui.active_tab || "",
          filesState: ui.files || null,
          aiSessionID: ui.ai_session_id || "",
          hydrated: true,
        });
        break;
      }
      case "clients": {
        const clients: ClientInfo[] = msg.clients || [];
        // 终端重命名/客户端改别名后，同步标签上的显示名
        const tabs = s.termTabs.map((t) => {
          const c = clients.find((x) => x.id === t.client_id);
          const term = c?.terminals?.[t.term_id];
          const cn = c ? c.alias || c.name : t.client_name;
          const changed = (term && term.title && term.title !== t.title) || cn !== t.client_name;
          return changed ? { ...t, title: term?.title || t.title, client_name: cn } : t;
        });
        useStore.setState({ clients, termTabs: tabs });
        break;
      }
      case "term_event": {
        if (msg.event === "closed") {
          const tabs = s.termTabs.map((t) =>
            t.client_id === msg.client_id && t.term_id === msg.term_id ? { ...t, closed: true } : t
          );
          useStore.setState({ termTabs: tabs });
          s.persistUI();
        }
        break;
      }
      case "ai_event":
        handleAIEvent(msg);
        break;
    }
  });
}

function handleAIEvent(msg: any) {
  const s = useStore.getState();
  const sessions = [...s.aiSessions];
  const idx = sessions.findIndex((x) => x.id === msg.session_id);
  switch (msg.kind) {
    case "session": {
      const i = sessions.findIndex((x) => x.id === msg.session.id);
      if (i >= 0) {
        sessions[i] = msg.session;
        useStore.setState({ aiSessions: sessions });
      } else {
        sessions.push(msg.session);
        // 新会话自动选中
        useStore.setState({ aiSessions: sessions, aiSessionID: msg.session.id });
        s.persistUI();
      }
      break;
    }
    case "session_deleted": {
      const rest = sessions.filter((x) => x.id !== msg.session_id);
      if (s.aiSessionID === msg.session_id) {
        // 当前会话被删（含批量删除）：自动切到剩余会话
        useStore.setState({ aiSessions: rest, aiSessionID: rest[rest.length - 1]?.id || "" });
        s.persistUI();
      } else {
        useStore.setState({ aiSessions: rest });
      }
      break;
    }
    case "msg":
      if (idx >= 0) {
        sessions[idx] = { ...sessions[idx], messages: [...(sessions[idx].messages || []), msg.message] };
        useStore.setState({ aiSessions: sessions });
      }
      break;
    case "status":
      if (idx >= 0) {
        sessions[idx] = { ...sessions[idx], status: msg.status };
        useStore.setState({ aiSessions: sessions });
      }
      break;
    case "approval":
      useStore.setState({
        approvals: [...s.approvals.filter((a) => a.id !== msg.approval.id), { ...msg.approval, session_id: msg.session_id }],
      });
      break;
  }
}
