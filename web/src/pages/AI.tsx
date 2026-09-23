import { useEffect, useRef, useState } from "react";
import { rest, ws } from "@/lib/api";
import { useStore } from "@/lib/store";
import type { AIMessage, AISession, AITarget, AIToolCall, ClientInfo } from "@/lib/types";
import { Badge, Button, Card, Input, Select } from "@/components/ui";
import { Dialog, DialogContent, DialogTitle } from "@/components/dialog";
import { Markdown } from "@/components/Markdown";
import { cn, clientName, formatTime, termLabel } from "@/lib/utils";
import { Bot, ListChecks, Plus, Send, Settings, Square, Trash2, User, Wrench, X } from "lucide-react";
import { useI18n } from "@/i18n";
import { errOf } from "@/i18n/errors";

const zh = {
  sessions: "AI 会话",
  exitBatch: "退出批量管理",
  batchManage: "批量管理",
  modelSettings: "模型设置",
  newSession: "新建会话",
  selected: "已选 {sel}/{total}",
  clearAll: "清空",
  selectAll: "全选",
  dblClickRename: "双击重命名",
  interrupted: "已中断",
  deleteSessionConfirm: "删除会话「{name}」？",
  noSessions: "还没有会话，点击 + 新建。",
  interruptN: "中断 {n} 个运行中的会话",
  noRunningSelected: "选中项没有运行中的会话",
  interrupt: "中断",
  delete: "删除",
  aiNotConfigured: "服务端未配置 AI（AI_BASE_URL / AI_API_KEY / AI_MODEL），配置后重启服务端生效。",
  fullAuto: "全自动",
  approvalMode: "审批模式",
  switchToApproval: "切换为审批",
  switchToAuto: "切换为全自动",
  switchToApprovalMode: "切换为审批模式",
  taskRunning: "任务执行中…",
  inputPlaceholder: "描述任务，例如：查看两台机器的系统信息",
  emptyHint: "选择或新建一个 AI 会话。AI 可直接操作选定客户端的终端（参照 hermes 的 agent 循环与授权机制）。",
  newAISession: "新建 AI 会话",
  approvalTitle: "AI 请求授权",
  deny: "拒绝",
  alwaysAllow: "始终允许此类",
  approve: "批准",
  aiSettingsTitle: "AI 模型设置",
  rename: "重命名",
  deleteSession: "删除会话",
  batchDeleteConfirm: "删除选中的 {n} 个会话？聊天记录将一并清除。",
  targetTerminals: "目标终端（{n}）",
  emptyTargetsHint: "为空表示不限定（AI 自行选择）",
  offline: "离线",
  noLiveTerminals: "（无存活终端）",
  sessionNameOptional: "会话名称（可选）",
  execMode: "执行模式：",
  approvalEach: "审批（每条命令确认）",
  cancel: "取消",
  create: "创建",
  saved: "✅ 已保存",
  testSuccess: "✅ 连接成功，模型回复：{reply}",
  baseUrlLabel: "接口地址（OpenAI 兼容，如 https://api.deepseek.com/v1）",
  apiKeyCurrent: "（当前：{key}，留空保持不变）",
  modelLabel: "模型",
  modelPlaceholder: "如 deepseek-v4-flash",
  defaultModeLabel: "新会话默认模式",
  envNote: "留空的字段将跟随服务端 .env 配置；保存在服务端并立即生效，无需重启。",
  testing: "测试中…",
  testConn: "测试连接",
  saving: "保存中…",
  save: "保存",
  callTool: "调用 {name}",
  toolResultOpen: "工具结果（",
  toolResultClose: "）",
  collapse: "收起",
  expand: "展开",
};
const en: Record<keyof typeof zh, string> = {
  sessions: "AI sessions",
  exitBatch: "Exit batch mode",
  batchManage: "Batch manage",
  modelSettings: "Model settings",
  newSession: "New session",
  selected: "Selected {sel}/{total}",
  clearAll: "Clear",
  selectAll: "Select all",
  dblClickRename: "Double-click to rename",
  interrupted: "Interrupted",
  deleteSessionConfirm: 'Delete session "{name}"?',
  noSessions: "No sessions yet. Click + to create one.",
  interruptN: "Interrupt {n} running session(s)",
  noRunningSelected: "No running sessions in selection",
  interrupt: "Interrupt",
  delete: "Delete",
  aiNotConfigured:
    "AI is not configured on the server (AI_BASE_URL / AI_API_KEY / AI_MODEL). Restart the server after configuring.",
  fullAuto: "Full auto",
  approvalMode: "Approval mode",
  switchToApproval: "Switch to approval",
  switchToAuto: "Switch to full auto",
  switchToApprovalMode: "Switch to approval mode",
  taskRunning: "Task running…",
  inputPlaceholder: "Describe a task, e.g. check system info on both machines",
  emptyHint:
    "Select or create an AI session. The AI can operate terminals of the selected clients directly (following the hermes agent loop and approval mechanism).",
  newAISession: "New AI session",
  approvalTitle: "AI approval request",
  deny: "Deny",
  alwaysAllow: "Always allow this",
  approve: "Approve",
  aiSettingsTitle: "AI model settings",
  rename: "Rename",
  deleteSession: "Delete session",
  batchDeleteConfirm: "Delete {n} selected session(s)? Chat history will also be removed.",
  targetTerminals: "Targets ({n})",
  emptyTargetsHint: "Empty means unrestricted (AI chooses on its own)",
  offline: "Offline",
  noLiveTerminals: "(no live terminals)",
  sessionNameOptional: "Session name (optional)",
  execMode: "Execution mode:",
  approvalEach: "Approval (confirm each command)",
  cancel: "Cancel",
  create: "Create",
  saved: "✅ Saved",
  testSuccess: "✅ Connected, model reply: {reply}",
  baseUrlLabel: "Base URL (OpenAI-compatible, e.g. https://api.deepseek.com/v1)",
  apiKeyCurrent: " (current: {key}, leave blank to keep unchanged)",
  modelLabel: "Model",
  modelPlaceholder: "e.g. deepseek-v4-flash",
  defaultModeLabel: "Default mode for new sessions",
  envNote:
    "Empty fields follow the server's .env config; settings are saved on the server and take effect immediately without restart.",
  testing: "Testing…",
  testConn: "Test connection",
  saving: "Saving…",
  save: "Save",
  callTool: "Call {name}",
  toolResultOpen: "Tool result (",
  toolResultClose: ")",
  collapse: "Collapse",
  expand: "Expand",
};
const dict = { zh, en };

// 从工具参数解析目标终端的展示标签，如 "→ web1:部署终端"
function targetLabel(clients: ClientInfo[], argsJson: string): string {
  try {
    const o = JSON.parse(argsJson);
    if (!o.client_id) return "";
    const c = clients.find((x) => x.id === o.client_id);
    return "→ " + termLabel(c, o.client_id, o.term_id || undefined);
  } catch {
    return "";
  }
}

export default function AI() {
  const { t } = useI18n(dict);
  const { clients, aiSessions, aiSessionID, approvals, aiDefaultMode, aiEnabled, set, persistUI } = useStore();
  const [input, setInput] = useState("");
  const [creating, setCreating] = useState(false);
  const [renamingSess, setRenamingSess] = useState(""); // 正在重命名的会话 id
  const [sessNameVal, setSessNameVal] = useState("");
  const [sessMenu, setSessMenu] = useState<{ x: number; y: number; s: AISession } | null>(null); // 会话右键菜单
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [batchMode, setBatchMode] = useState(false);          // 会话批量管理模式
  const [batchSel, setBatchSel] = useState<Set<string>>(new Set()); // 批量勾选的会话 id
  const bottomRef = useRef<HTMLDivElement>(null);

  const session = aiSessions.find((s) => s.id === aiSessionID);
  const pendingApprovals = approvals.filter((a) => a.session_id === aiSessionID);

  // ---- 会话批量管理 ----
  const toggleBatch = (id: string) => {
    const next = new Set(batchSel);
    if (next.has(id)) next.delete(id); else next.add(id);
    setBatchSel(next);
  };
  const selSessions = aiSessions.filter((s) => batchSel.has(s.id));
  const selRunning = selSessions.filter((s) => s.status === "running").length;
  const batchInterrupt = () => {
    for (const s of selSessions) {
      if (s.status === "running") ws.send({ type: "ai_interrupt", session_id: s.id });
    }
    setBatchSel(new Set());
  };
  const batchDelete = () => {
    if (!selSessions.length) return;
    if (!confirm(t("batchDeleteConfirm", { n: selSessions.length }))) return;
    for (const s of selSessions) ws.send({ type: "ai_delete", session_id: s.id });
    setBatchSel(new Set());
    setBatchMode(false);
  };

  // 工具调用 id → 调用参数（用于在工具结果上展示目标终端）
  const callById = new Map<string, AIToolCall>();
  (session?.messages || []).forEach((m) => (m.tool_calls || []).forEach((tc) => callById.set(tc.id, tc)));

  const commitSessRename = (id: string) => {
    const name = sessNameVal.trim();
    setRenamingSess("");
    const s = aiSessions.find((x) => x.id === id);
    if (name && s && name !== s.name) ws.send({ type: "ai_rename", session_id: id, name });
  };

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [session?.messages?.length]);

  const send = () => {
    if (!input.trim() || !session) return;
    ws.send({ type: "ai_msg", session_id: session.id, content: input });
    setInput("");
  };

  const approve = (approvalID: string, decision: string) => {
    ws.send({ type: "ai_approve", session_id: aiSessionID, approval_id: approvalID, decision });
    useStore.setState({ approvals: approvals.filter((a) => a.id !== approvalID) });
  };

  return (
    <div className="flex h-full">
      {/* 会话列表 */}
      <div className="w-56 shrink-0 border-r border-border flex flex-col">
        <div className="flex items-center justify-between p-2 border-b border-border">
          <span className="text-sm font-medium">{t("sessions")}</span>
          <div className="flex">
            <Button
              size="icon" variant="ghost"
              data-testid="ai-batch-btn"
              title={batchMode ? t("exitBatch") : t("batchManage")}
              className={cn(batchMode && "bg-accent")}
              onClick={() => { setBatchMode(!batchMode); setBatchSel(new Set()); }}
            >
              <ListChecks size={16} />
            </Button>
            <Button size="icon" variant="ghost" data-testid="btn-ai-settings" onClick={() => setSettingsOpen(true)} title={t("modelSettings")}><Settings size={16} /></Button>
            <Button size="icon" variant="ghost" onClick={() => setCreating(true)} title={t("newSession")}><Plus size={16} /></Button>
          </div>
        </div>
        {batchMode && (
          <div className="flex items-center justify-between border-b border-border px-2 py-1 text-xs text-muted-foreground">
            <span>{t("selected", { sel: selSessions.length, total: aiSessions.length })}</span>
            <button
              data-testid="ai-batch-all"
              className="text-primary hover:underline"
              onClick={() => setBatchSel(selSessions.length === aiSessions.length && aiSessions.length > 0 ? new Set() : new Set(aiSessions.map((s) => s.id)))}
            >
              {selSessions.length === aiSessions.length && aiSessions.length > 0 ? t("clearAll") : t("selectAll")}
            </button>
          </div>
        )}
        <div className="flex-1 overflow-auto">
          {aiSessions.map((s) => (
            <div
              key={s.id}
              data-testid={batchMode ? "ai-batch-item" : undefined}
              className={cn(
                "group flex items-center gap-1 px-3 py-2 text-sm cursor-pointer hover:bg-accent",
                s.id === aiSessionID && !batchMode && "bg-accent",
                batchMode && batchSel.has(s.id) && "bg-accent"
              )}
              onClick={() => {
                if (batchMode) { toggleBatch(s.id); return; }
                set({ aiSessionID: s.id }); persistUI();
              }}
              onContextMenu={(e) => {
                if (batchMode) return;
                e.preventDefault();
                setSessMenu({ x: Math.min(e.clientX, window.innerWidth - 200), y: Math.min(e.clientY, window.innerHeight - 170), s });
              }}
            >
              {batchMode ? (
                <input type="checkbox" readOnly checked={batchSel.has(s.id)} className="shrink-0" />
              ) : (
                <Bot size={14} className="shrink-0" />
              )}
              {renamingSess === s.id ? (
                <input
                  data-testid="ai-rename-input"
                  autoFocus
                  className="h-6 flex-1 min-w-0 rounded border border-primary bg-input px-1 text-sm outline-none"
                  value={sessNameVal}
                  onChange={(e) => setSessNameVal(e.target.value)}
                  onClick={(e) => e.stopPropagation()}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") commitSessRename(s.id);
                    if (e.key === "Escape") setRenamingSess("");
                  }}
                  onBlur={() => commitSessRename(s.id)}
                />
              ) : (
                <span
                  className="flex-1 truncate"
                  title={batchMode ? s.name : t("dblClickRename")}
                  onDoubleClick={(e) => {
                    if (batchMode) return;
                    e.stopPropagation(); setRenamingSess(s.id); setSessNameVal(s.name);
                  }}
                >
                  {s.name}
                </span>
              )}
              {s.status === "running" && <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />}
              {s.status === "interrupted" && <span className="h-2 w-2 rounded-full bg-yellow-500" title={t("interrupted")} />}
              {!batchMode && (
                <button
                  className="opacity-0 group-hover:opacity-100"
                  onClick={(e) => {
                    e.stopPropagation();
                    if (confirm(t("deleteSessionConfirm", { name: s.name }))) ws.send({ type: "ai_delete", session_id: s.id });
                  }}
                >
                  <Trash2 size={13} />
                </button>
              )}
            </div>
          ))}
          {aiSessions.length === 0 && <div className="p-3 text-xs text-muted-foreground">{t("noSessions")}</div>}
        </div>
        {batchMode && (
          <div className="flex gap-1 border-t border-border p-1.5">
            <Button
              size="sm" variant="outline"
              data-testid="ai-batch-interrupt"
              disabled={!selRunning}
              title={selRunning ? t("interruptN", { n: selRunning }) : t("noRunningSelected")}
              onClick={batchInterrupt}
            >
              <Square size={12} /> {t("interrupt")}{selRunning ? `(${selRunning})` : ""}
            </Button>
            <Button
              size="sm" variant="destructive" className="flex-1"
              data-testid="ai-batch-delete"
              disabled={!selSessions.length}
              onClick={batchDelete}
            >
              <Trash2 size={12} /> {t("delete")}{selSessions.length ? `(${selSessions.length})` : ""}
            </Button>
            <Button size="sm" variant="ghost" title={t("exitBatch")} onClick={() => { setBatchMode(false); setBatchSel(new Set()); }}>
              <X size={12} />
            </Button>
          </div>
        )}
      </div>

      {/* 聊天区 */}
      <div className="flex-1 flex flex-col min-w-0">
        {!aiEnabled && (
          <div className="bg-yellow-500/10 text-yellow-600 dark:text-yellow-400 text-sm px-4 py-2">
            {t("aiNotConfigured")}
          </div>
        )}
        {session ? (
          <>
            <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-sm flex-wrap">
              <span className="font-medium">{session.name}</span>
              <Badge variant={session.mode === "auto" ? "destructive" : "default"}>
                {session.mode === "auto" ? t("fullAuto") : t("approvalMode")}
              </Badge>
              <Button
                size="sm" variant="outline"
                onClick={() => ws.send({ type: "ai_set_mode", session_id: session.id, mode: session.mode === "auto" ? "approval" : "auto" })}
              >
                {session.mode === "auto" ? t("switchToApproval") : t("switchToAuto")}
              </Button>
              <TargetPicker />
              {session.status === "running" && (
                <Button size="sm" variant="destructive" onClick={() => ws.send({ type: "ai_interrupt", session_id: session.id })}>
                  <Square size={13} /> {t("interrupt")}
                </Button>
              )}
            </div>
            <div className="flex-1 overflow-auto p-4 space-y-3">
              {(session.messages || []).filter((m) => m.role !== "system" || m.content.startsWith("[")).map((m, i) => (
                <MessageView key={i} m={m} calls={callById} />
              ))}
              <div ref={bottomRef} />
            </div>
            <div className="border-t border-border p-3 flex gap-2">
              <Input
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && !e.shiftKey && (e.preventDefault(), send())}
                placeholder={session.status === "running" ? t("taskRunning") : t("inputPlaceholder")}
                disabled={session.status === "running"}
              />
              <Button onClick={send} disabled={!input.trim() || session.status === "running"}><Send size={15} /></Button>
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-sm text-muted-foreground">
            {t("emptyHint")}
          </div>
        )}
      </div>

      {/* 新建会话 */}
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent>
          <DialogTitle>{t("newAISession")}</DialogTitle>
          <NewSessionForm
            defaultMode={aiDefaultMode}
            onDone={() => setCreating(false)}
          />
        </DialogContent>
      </Dialog>

      {/* 审批弹窗 */}
      {pendingApprovals.map((a) => (
        <Dialog key={a.id} open onOpenChange={() => {}}>
          <DialogContent>
            <DialogTitle>{t("approvalTitle")}</DialogTitle>
            <div className="text-sm mb-2">{a.summary}</div>
            <pre className="max-h-48 overflow-auto rounded bg-muted p-2 text-xs mb-4">{tryPretty(a.args)}</pre>
            <div className="flex justify-end gap-2">
              <Button variant="destructive" onClick={() => approve(a.id, "deny")}>{t("deny")}</Button>
              <Button variant="outline" onClick={() => approve(a.id, "allow_always")}>{t("alwaysAllow")}</Button>
              <Button onClick={() => approve(a.id, "allow")}>{t("approve")}</Button>
            </div>
          </DialogContent>
        </Dialog>
      ))}

      {/* 模型设置 */}
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent>
          <DialogTitle>{t("aiSettingsTitle")}</DialogTitle>
          <AISettingsForm onClose={() => setSettingsOpen(false)} />
        </DialogContent>
      </Dialog>

      {/* 会话右键菜单 */}
      {sessMenu && (
        <div
          className="fixed inset-0 z-50"
          onClick={() => setSessMenu(null)}
          onContextMenu={(e) => { e.preventDefault(); setSessMenu(null); }}
        >
          <Card
            data-testid="ai-sess-menu"
            className="absolute w-48 p-1 text-sm shadow-lg"
            style={{ left: sessMenu.x, top: sessMenu.y }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-2 py-1 text-xs text-muted-foreground truncate">{sessMenu.s.name}</div>
            <button
              data-testid="ai-menu-rename"
              className="flex w-full items-center rounded px-2 py-1.5 text-left hover:bg-accent"
              onClick={() => { setRenamingSess(sessMenu.s.id); setSessNameVal(sessMenu.s.name); setSessMenu(null); }}
            >
              {t("rename")}
            </button>
            <button
              className="flex w-full items-center rounded px-2 py-1.5 text-left hover:bg-accent"
              onClick={() => {
                ws.send({ type: "ai_set_mode", session_id: sessMenu.s.id, mode: sessMenu.s.mode === "auto" ? "approval" : "auto" });
                setSessMenu(null);
              }}
            >
              {sessMenu.s.mode === "auto" ? t("switchToApprovalMode") : t("switchToAuto")}
            </button>
            <button
              className="flex w-full items-center rounded px-2 py-1.5 text-left text-red-400 hover:bg-accent"
              onClick={() => {
                if (confirm(t("deleteSessionConfirm", { name: sessMenu.s.name }))) ws.send({ type: "ai_delete", session_id: sessMenu.s.id });
                setSessMenu(null);
              }}
            >
              {t("deleteSession")}
            </button>
          </Card>
        </div>
      )}
    </div>
  );
}

function tryPretty(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

// 目标选择器：选择会话可操作的终端（多选，精确到某客户端的某终端）。
function TargetPicker() {
  const { t } = useI18n(dict);
  const { clients, aiSessions, aiSessionID } = useStore();
  const [open, setOpen] = useState(false);
  const session = aiSessions.find((s) => s.id === aiSessionID);
  if (!session) return null;

  const has = (cid: string, tid: string) => session.targets?.some((t) => t.client_id === cid && t.term_id === tid);

  const toggle = (cid: string, tid: string) => {
    const c = clients.find((x) => x.id === cid);
    const term = Object.values(c?.terminals || {}).find((t) => t.id === tid);
    if (!c || !term) return;
    let targets: AITarget[];
    if (has(cid, tid)) {
      targets = session.targets.filter((t) => !(t.client_id === cid && t.term_id === tid));
    } else {
      targets = [...(session.targets || []), { client_id: cid, term_id: tid, client_name: clientName(c), title: term.title }];
    }
    ws.send({ type: "ai_set_targets", session_id: session.id, targets });
  };

  const selectAll = () => {
    const targets: AITarget[] = [];
    for (const c of clients) {
      if (!c.online) continue;
      for (const t of Object.values(c.terminals || {})) {
        if (!t.closed) targets.push({ client_id: c.id, term_id: t.id, client_name: clientName(c), title: t.title });
      }
    }
    ws.send({ type: "ai_set_targets", session_id: session.id, targets });
  };

  return (
    <div className="relative">
      <Button size="sm" variant="outline" onClick={() => setOpen(!open)}>
        {t("targetTerminals", { n: session.targets?.length || 0 })}
      </Button>
      {open && (
        <Card className="absolute z-40 mt-1 w-72 max-h-80 overflow-auto p-2 text-sm">
          <div className="flex justify-between items-center mb-1">
            <span className="text-xs text-muted-foreground">{t("emptyTargetsHint")}</span>
            <Button size="sm" variant="ghost" onClick={selectAll}>{t("selectAll")}</Button>
          </div>
          {clients.map((c) => (
            <div key={c.id} className="mb-1">
              <div className="font-medium flex items-center gap-1">
                {clientName(c)} {!c.online && <Badge variant="destructive">{t("offline")}</Badge>}
              </div>
              <div className="pl-3">
                {Object.values(c.terminals || {}).filter((t) => !t.closed).map((t) => (
                  <label key={t.id} className="flex items-center gap-2 py-0.5 cursor-pointer">
                    <input type="checkbox" checked={!!has(c.id, t.id)} onChange={() => toggle(c.id, t.id)} />
                    {t.title} <span className="text-xs text-muted-foreground">({t.id})</span>
                  </label>
                ))}
                {Object.values(c.terminals || {}).filter((t) => !t.closed).length === 0 && (
                  <div className="text-xs text-muted-foreground">{t("noLiveTerminals")}</div>
                )}
              </div>
            </div>
          ))}
        </Card>
      )}
    </div>
  );
}

function NewSessionForm({ defaultMode, onDone }: { defaultMode: string; onDone: () => void }) {
  const { t } = useI18n(dict);
  const [name, setName] = useState("");
  const [mode, setMode] = useState(defaultMode === "auto" ? "auto" : "approval");
  return (
    <div className="space-y-3">
      <Input placeholder={t("sessionNameOptional")} value={name} onChange={(e) => setName(e.target.value)} />
      <div className="flex items-center gap-3 text-sm">
        <span className="text-muted-foreground">{t("execMode")}</span>
        <label className="flex items-center gap-1">
          <input type="radio" checked={mode === "approval"} onChange={() => setMode("approval")} /> {t("approvalEach")}
        </label>
        <label className="flex items-center gap-1">
          <input type="radio" checked={mode === "auto"} onChange={() => setMode("auto")} /> {t("fullAuto")}
        </label>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onDone}>{t("cancel")}</Button>
        <Button onClick={() => { ws.send({ type: "ai_create", name, mode }); onDone(); }}>{t("create")}</Button>
      </div>
    </div>
  );
}

// AI 模型设置表单：配置保存在服务端（覆盖 .env），所有浏览器/App 端共享生效。
function AISettingsForm({ onClose }: { onClose: () => void }) {
  const { lang, t } = useI18n(dict);
  const [form, setForm] = useState({ base_url: "", api_key: "", model: "", default_mode: "approval" });
  const [keyMask, setKeyMask] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState("");

  useEffect(() => {
    rest.aiConfig()
      .then((d) => {
        setForm({ base_url: d.base_url || "", api_key: "", model: d.model || "", default_mode: d.default_mode || "approval" });
        setKeyMask(d.api_key_set ? d.api_key_mask : "");
      })
      .catch((e) => setMsg("❌ " + errOf(e, lang)));
  }, []);

  const applySaved = (d: any) => {
    useStore.setState({ aiEnabled: !!d.ai_enabled, aiDefaultMode: d.ai_default_mode || "approval" });
    if (d.api_key_mask) setKeyMask(d.api_key_mask);
    setForm((f) => ({ ...f, api_key: "" }));
  };

  const save = async (): Promise<boolean> => {
    const d = await rest.saveAIConfig(form);
    applySaved(d);
    return true;
  };

  const onSave = async () => {
    setBusy("save"); setMsg("");
    try {
      await save();
      setMsg(t("saved"));
      setTimeout(onClose, 600);
    } catch (e: any) {
      setMsg("❌ " + errOf(e, lang));
    } finally {
      setBusy("");
    }
  };

  const onTest = async () => {
    setBusy("test"); setMsg("");
    try {
      await save(); // 先保存，确保测的是表单里的配置
      const d = await rest.testAI();
      setMsg(t("testSuccess", { reply: String(d.reply || "").slice(0, 60) }));
    } catch (e: any) {
      setMsg("❌ " + errOf(e, lang));
    } finally {
      setBusy("");
    }
  };

  const field = "text-xs text-muted-foreground";
  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <div className={field}>{t("baseUrlLabel")}</div>
        <Input data-testid="ai-cfg-base" placeholder="AI_BASE_URL" value={form.base_url}
          onChange={(e) => setForm({ ...form, base_url: e.target.value })} />
      </div>
      <div className="space-y-1">
        <div className={field}>API Key{keyMask ? t("apiKeyCurrent", { key: keyMask }) : ""}</div>
        <Input data-testid="ai-cfg-key" type="password" placeholder={keyMask || "AI_API_KEY"} value={form.api_key}
          onChange={(e) => setForm({ ...form, api_key: e.target.value })} />
      </div>
      <div className="space-y-1">
        <div className={field}>{t("modelLabel")}</div>
        <Input data-testid="ai-cfg-model" placeholder={t("modelPlaceholder")} value={form.model}
          onChange={(e) => setForm({ ...form, model: e.target.value })} />
      </div>
      <div className="space-y-1">
        <div className={field}>{t("defaultModeLabel")}</div>
        <Select data-testid="ai-cfg-mode" value={form.default_mode}
          onChange={(e) => setForm({ ...form, default_mode: e.target.value })}>
          <option value="approval">{t("approvalEach")}</option>
          <option value="auto">{t("fullAuto")}</option>
        </Select>
      </div>
      <div className="text-xs text-muted-foreground">{t("envNote")}</div>
      {msg && <div className="text-sm">{msg}</div>}
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onTest} disabled={!!busy}>
          {busy === "test" ? t("testing") : t("testConn")}
        </Button>
        <Button data-testid="ai-cfg-save" onClick={onSave} disabled={!!busy}>
          {busy === "save" ? t("saving") : t("save")}
        </Button>
      </div>
    </div>
  );
}

function MessageView({ m, calls }: { m: AIMessage; calls?: Map<string, AIToolCall> }) {
  const { lang, t } = useI18n(dict);
  const [expand, setExpand] = useState(false);
  const clients = useStore((s) => s.clients);
  const time = m.time ? (
    <div className="mt-0.5 text-[10px] text-muted-foreground">{formatTime(m.time, lang)}</div>
  ) : null;
  if (m.role === "user") {
    return (
      <div className="flex flex-col items-end">
        <div className="max-w-[80%] rounded-lg bg-primary text-primary-foreground px-3 py-2 text-sm whitespace-pre-wrap">
          <User size={12} className="inline mr-1" />{m.content}
        </div>
        {time}
      </div>
    );
  }
  if (m.role === "assistant") {
    return (
      <div className="max-w-[85%] space-y-1">
        {m.content && (
          <div className="rounded-lg bg-muted px-3 py-2 text-sm">
            <Bot size={12} className="inline mr-1" />
            <Markdown>{m.content}</Markdown>
          </div>
        )}
        {(m.tool_calls || []).map((tc) => (
          <div key={tc.id} className="flex items-center gap-1 text-xs text-muted-foreground">
            <Wrench size={11} /> {t("callTool", { name: tc.name })}
            <span className="text-blue-400">{targetLabel(clients, tc.arguments)}</span>
            <code className="bg-muted rounded px-1 truncate max-w-[420px]">{tryInline(tc.arguments)}</code>
          </div>
        ))}
        {time}
      </div>
    );
  }
  if (m.role === "tool") {
    const long = m.content.length > 300;
    const call = m.tool_call_id ? calls?.get(m.tool_call_id) : undefined;
    return (
      <div className="max-w-[85%]">
        <div className="cursor-pointer rounded-lg border border-border px-3 py-1.5 text-xs" onClick={() => setExpand(!expand)}>
          <span className="text-muted-foreground">
            {t("toolResultOpen")}{m.name}
            {call && <span className="text-blue-400"> {targetLabel(clients, call.arguments)}</span>}{t("toolResultClose")}
            {m.time ? ` · ${formatTime(m.time, lang)}` : ""}
          </span>
          <pre className={cn("whitespace-pre-wrap break-all mt-1", !expand && long && "line-clamp-3")}>
            {m.content}
          </pre>
          {long && <span className="text-blue-500">{expand ? t("collapse") : t("expand")}</span>}
        </div>
      </div>
    );
  }
  // system 提示（中断/错误等）
  return (
    <div className="text-center text-xs text-muted-foreground">
      {m.content} {m.time ? `· ${formatTime(m.time, lang)}` : ""}
    </div>
  );
}

function tryInline(s: string): string {
  try {
    const o = JSON.parse(s);
    if (o.command) return o.command;
    if (o.path) return o.path;
    return s;
  } catch {
    return s;
  }
}
