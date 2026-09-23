import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { rest, ws } from "@/lib/api";
import { useStore } from "@/lib/store";
import type { ClientInfo } from "@/lib/types";
import { Badge, Button, Card } from "@/components/ui";
import { Dialog, DialogContent, DialogTitle } from "@/components/dialog";
import { AttachRows, ShellRows, hasAttachable } from "@/components/TermMenus";
import { clientName, formatTime } from "@/lib/utils";
import { ChevronDown, FileText, Monitor, Pencil, Trash2 } from "lucide-react";
import { useI18n } from "@/i18n";
import { errOf } from "@/i18n/errors";

const zh = {
  title: "客户端",
  setAlias: "设置别名",
  online: "在线",
  offline: "离线",
  clientMeta: "ID: {id} · 终端 {count} 个 · 最近在线 {lastSeen}",
  terminal: "终端",
  newTerminalSelectType: "新建终端（选择类型）",
  attachExisting: "接入已有终端",
  files: "文件",
  delete: "删除",
  noClients: "暂无客户端接入。请在目标机器上运行 termirai-agent。",
  deleteClient: "删除客户端",
  confirmDeleteHead: "确定删除离线客户端",
  confirmDeleteTail: "（{id}）？其终端记录将一并清除。",
  cancel: "取消",
};
const en: Record<keyof typeof zh, string> = {
  title: "Clients",
  setAlias: "Set alias",
  online: "Online",
  offline: "Offline",
  clientMeta: "ID: {id} · Terminals: {count} · Last seen {lastSeen}",
  terminal: "Terminal",
  newTerminalSelectType: "New terminal (choose type)",
  attachExisting: "Attach to existing terminal",
  files: "Files",
  delete: "Delete",
  noClients: "No clients connected. Run termirai-agent on the target machine.",
  deleteClient: "Delete client",
  confirmDeleteHead: "Delete offline client",
  confirmDeleteTail: " ({id})? Its terminal records will be removed as well.",
  cancel: "Cancel",
};
const dict = { zh, en };

export default function Clients() {
  const { lang, t } = useI18n(dict);
  const { clients, termTabs, set, persistUI } = useStore();
  const [delTarget, setDelTarget] = useState<ClientInfo | null>(null);
  const [err, setErr] = useState("");
  const [editingAlias, setEditingAlias] = useState(""); // 正在编辑别名的 client id
  const [aliasVal, setAliasVal] = useState("");
  const [termMenuFor, setTermMenuFor] = useState(""); // 打开终端下拉面板的 client id
  const nav = useNavigate();
  const tabKeys = new Set(termTabs.map((t) => t.key));

  const commitAlias = (c: ClientInfo) => {
    const alias = aliasVal.trim();
    setEditingAlias("");
    if (alias !== (c.alias || "")) {
      ws.send({ type: "client_alias", client_id: c.id, name: alias });
    }
  };

  // 跳转终端页并执行动作：新建（可指定 shell）或接入已有终端（pendingTerm 由终端页消费）
  const gotoTerminal = (p: { client_id: string; term_id?: string; title?: string; shell?: string }) => {
    set({ page: "terminals", pendingTerm: p });
    persistUI();
    setTermMenuFor("");
    nav("/terminals");
  };
  const openFiles = (c: ClientInfo) => {
    set({ filesState: { client_id: c.id, path: "/" }, page: "files" });
    persistUI();
    nav("/files");
  };

  const doDelete = async () => {
    if (!delTarget) return;
    try {
      await rest.deleteClient(delTarget.id);
      setDelTarget(null);
      setErr("");
    } catch (e: any) {
      setErr(errOf(e, lang));
    }
  };

  return (
    <div className="p-4 space-y-3">
      <h2 className="text-lg font-semibold">{t("title")}</h2>
      {err && <div className="text-sm text-red-500">{err}</div>}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {clients.map((c) => {
          const terms = Object.values(c.terminals || {});
          const alive = terms.filter((t) => !t.closed);
          return (
            <Card key={c.id} className="p-4 space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-1 min-w-0">
                  {editingAlias === c.id ? (
                    <input
                      data-testid="alias-input"
                      autoFocus
                      className="h-7 w-36 rounded border border-primary bg-input px-1.5 text-sm outline-none"
                      placeholder={c.name}
                      value={aliasVal}
                      onChange={(e) => setAliasVal(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") commitAlias(c);
                        if (e.key === "Escape") setEditingAlias("");
                      }}
                      onBlur={() => commitAlias(c)}
                    />
                  ) : (
                    <>
                      <span className="font-medium truncate" title={c.name}>{clientName(c)}</span>
                      <button
                        data-testid="btn-edit-alias"
                        className="shrink-0 rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                        title={t("setAlias")}
                        onClick={() => { setEditingAlias(c.id); setAliasVal(c.alias || ""); }}
                      >
                        <Pencil size={12} />
                      </button>
                    </>
                  )}
                </div>
                {c.online ? <Badge variant="success">{t("online")}</Badge> : <Badge variant="destructive">{t("offline")}</Badge>}
              </div>
              <div className="text-xs text-muted-foreground">
                {c.alias ? `${c.name} · ` : ""}{c.host}
              </div>
              <div className="text-xs text-muted-foreground">
                {t("clientMeta", { id: c.id, count: alive.length, lastSeen: c.last_seen ? formatTime(c.last_seen, lang) : "-" })}
              </div>
              <div className="flex gap-2 pt-1">
                <div className="relative">
                  <Button
                    size="sm"
                    data-testid="client-term-btn"
                    disabled={!c.online}
                    onClick={() => setTermMenuFor(termMenuFor === c.id ? "" : c.id)}
                  >
                    <Monitor size={14} /> {t("terminal")} <ChevronDown size={12} />
                  </Button>
                  {termMenuFor === c.id && (
                    <>
                      <div className="fixed inset-0 z-40" onClick={() => setTermMenuFor("")} />
                      <Card
                        data-testid="client-term-menu"
                        className="absolute left-0 top-full z-50 mt-1 max-h-72 w-60 overflow-y-auto p-1 text-sm shadow-lg"
                      >
                        <div className="px-2 py-1 text-xs text-muted-foreground">{t("newTerminalSelectType")}</div>
                        <ShellRows
                          client={c}
                          onPick={(shell) => gotoTerminal({ client_id: c.id, shell })}
                        />
                        {hasAttachable(c, tabKeys) && (
                          <>
                            <div className="mt-1 border-t border-border px-2 py-1 text-xs text-muted-foreground">{t("attachExisting")}</div>
                            <AttachRows
                              client={c}
                              excludeKeys={tabKeys}
                              onPick={(t) => gotoTerminal({ client_id: c.id, term_id: t.id, title: t.title })}
                            />
                          </>
                        )}
                      </Card>
                    </>
                  )}
                </div>
                <Button size="sm" variant="outline" disabled={!c.online} onClick={() => openFiles(c)}>
                  <FileText size={14} /> {t("files")}
                </Button>
                {!c.online && (
                  <Button size="sm" variant="destructive" onClick={() => setDelTarget(c)}>
                    <Trash2 size={14} /> {t("delete")}
                  </Button>
                )}
              </div>
            </Card>
          );
        })}
        {clients.length === 0 && (
          <div className="text-sm text-muted-foreground">{t("noClients")}</div>
        )}
      </div>

      <Dialog open={!!delTarget} onOpenChange={() => setDelTarget(null)}>
        <DialogContent>
          <DialogTitle>{t("deleteClient")}</DialogTitle>
          <p className="text-sm text-muted-foreground mb-4">
            {t("confirmDeleteHead")} <b>{delTarget?.name}</b>{t("confirmDeleteTail", { id: delTarget?.id ?? "" })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setDelTarget(null)}>{t("cancel")}</Button>
            <Button variant="destructive" onClick={doDelete}>{t("delete")}</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
