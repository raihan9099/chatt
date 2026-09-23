// 与服务端对齐的类型定义。
export interface TermInfo {
  id: string;
  title: string;
  created_at: number;
  closed: boolean;
}

export interface ClientInfo {
  id: string;
  name: string;
  alias?: string;
  host: string;
  version: string;
  last_seen: number;
  online: boolean;
  shells?: ShellInfo[];
  terminals: Record<string, TermInfo>;
}

export interface ShellInfo {
  name: string;
  path: string;
}

export interface AITarget {
  client_id: string;
  term_id: string;
  client_name: string;
  title: string;
}

export interface AIToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface AIMessage {
  role: string;
  content: string;
  tool_calls?: AIToolCall[];
  tool_call_id?: string;
  name?: string;
  time: number;
}

export interface AISession {
  id: string;
  name: string;
  targets: AITarget[];
  mode: string;
  messages: AIMessage[];
  allowlist: string[];
  status: string;
  created_at: number;
  updated_at: number;
}

export interface Approval {
  id: string;
  tool: string;
  args: string;
  summary: string;
  session_id: string;
}

export interface FileEntry {
  name: string;
  path: string;
  dir: boolean;
  size: number;
  mode: string;
  mtime: number;
}

// 打开的终端标签（页面状态的一部分，会被持久化到服务端）。
export interface TermTab {
  key: string; // clientID/termID
  client_id: string;
  term_id: string;
  client_name: string;
  title: string;
  closed?: boolean;
}

// 持久化的页面状态。
export interface UIState {
  page?: string; // clients | terminals | files | ai
  term_tabs?: TermTab[];
  active_tab?: string;
  files?: { client_id: string; path: string; open_file?: string };
  ai_session_id?: string;
}
