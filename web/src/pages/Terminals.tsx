import { useEffect, useState } from "react";
import { ws } from "@/lib/api";
import { useStore } from "@/lib/store";
import type { ClientInfo, TermTab } from "@/lib/types";
import { Button, Card } from "@/components/ui";
import { TermView } from "@/components/Terminal";
import { AttachRows, ShellRows, hasAttachable } from "@/components/TermMenus";
import { ChevronDown, ChevronRight, ListChecks, Plus, X } from "lucide-react";
import { cn, clientName } from "@/lib/utils";
import { useI18n } from "@/i18n";

const zh = {
  defaultTitle: "终端",
  tabTooltip: "{client}:{title}（双击重命名，右键更多操作）",
  closeDestroy: "关闭终端（销毁）",
  removeFromPage: "从页面移除",
  batchManage: "批量管理",
  batchSummary: "批量管理（存活终端 {total} 个，已选 {sel}）",
  newTerm: "新建终端",
  clearSel: "清空",
  selectAll: "全选",
  offline: "（离线）",
  inPage: "已在页面",
  notInPage: "未接入",
  noAliveTerms: "暂无存活终端",
  attachToPage: "接入页面",
  closeTerm: "关闭终端",
  newTermHint: "新建终端（选择客户端与类型）",
  noOnlineClients: "暂无在线客户端",
  attachExisting: "接入已有终端",
  termClosed: "该终端已关闭。",
  removeTab: "移除标签",
  emptyHint: "选择客户端打开终端；关闭浏览器后终端仍在客户端运行，重新登录即可回到现场。",
  rename: "重命名",
  removeKeep: "从页面移除（终端保留）",
};
const en: Record<keyof typeof zh, string> = {
  defaultTitle: "Terminal",
  tabTooltip: "{client}:{title} (double-click to rename, right-click for more)",
  closeDestroy: "Close terminal (destroy)",
  removeFromPage: "Remove from page",
  batchManage: "Batch manage",
  batchSummary: "Batch manage ({total} alive, {sel} selected)",
  newTerm: "New terminal",
  clearSel: "Clear",
  selectAll: "Select all",
  offline: " (offline)",
  inPage: "In page",
  notInPage: "Not in page",
  noAliveTerms: "No alive terminals",
  attachToPage: "Attach to page",
  closeTerm: "Close terminal",
  newTermHint: "New terminal (choose a client and type)",
  noOnlineClients: "No online clients",
  attachExisting: "Attach existing terminals",
  termClosed: "This terminal is closed.",
  removeTab: "Remove tab",
  emptyHint: "Select a client to open a terminal; terminals keep running on the client after the browser is closed, sign in again to return.",
  rename: "Rename",
  removeKeep: "Remove from page (keeps terminal)",
};
const dict = { zh, en };

export default function Terminals() {
  const { t } = useI18n(dict);
  const { clients, termTabs, activeTab, pendingTerm, set, persistUI } = useStore();
  const [newOpen, setNewOpen] = useState(false);       // 新建终端面板
  const [expandClient, setExpandClient] = useState(""); // 面板中展开 shell 列表的客户端 id
  const [batchOpen, setBatchOpen] = useState(false);   // 批量管理面板
  const [batchSel, setBatchSel] = useState<Set<string>>(new Set()); // 批量勾选的终端 key
  const [renaming, setRenaming] = useState(""); // 正在重命名的 tab.key
  const [renameVal, setRenameVal] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number; tab: TermTab } | null>(null); // 右键菜单

  const onlineClients = clients.filter((c) => c.online);

  // 消费客户端页跳转请求：按载荷新建（可带 shell）或接入该客户端的终端
  useEffect(() => {
    if (!pendingTerm) return;
    const c = clients.find((x) => x.id === pendingTerm.client_id);
    if (c) openTerm(c, pendingTerm.term_id || "", pendingTerm.title || "", pendingTerm.shell || "");
    set({ pendingTerm: null });
  }, [pendingTerm, clients]);

  const openTerm = (c: ClientInfo, termID = "", title = "", shell = "") => {
    const cols = 100, rows = 30;
    ws.send({ type: "term_open", client_id: c.id, term_id: termID, title: title || t("defaultTitle"), cols, rows, shell });
  };

  // term_opened → 加标签
  useEffect(() => {
    return ws.subscribe((msg) => {
      if (msg.type === "term_opened") {
        const { termTabs, clients, set, persistUI } = useStore.getState();
        const c = clients.find((x) => x.id === msg.client_id);
        const key = `${msg.client_id}/${msg.term_id}`;
        if (!termTabs.find((t) => t.key === key)) {
          const tab: TermTab = {
            key,
            client_id: msg.client_id,
            term_id: msg.term_id,
            client_name: clientName(c) || msg.client_id,
            title: msg.title || t("defaultTitle"),
          };
          set({ termTabs: [...termTabs, tab], activeTab: key });
          persistUI();
        } else {
          set({ activeTab: key });
          persistUI();
        }
      }
    });
  }, [t]);

  const closeTab = (tab: TermTab) => {
    ws.send({ type: "term_close", client_id: tab.client_id, term_id: tab.term_id });
    const tabs = termTabs.filter((t) => t.key !== tab.key);
    set({ termTabs: tabs, activeTab: activeTab === tab.key ? tabs[tabs.length - 1]?.key || "" : activeTab });
    persistUI();
  };

  const detachTab = (tab: TermTab) => {
    // 仅从页面移除，不关闭终端（终端常驻）
    const tabs = termTabs.filter((t) => t.key !== tab.key);
    set({ termTabs: tabs, activeTab: activeTab === tab.key ? tabs[tabs.length - 1]?.key || "" : activeTab });
    persistUI();
  };

  const commitRename = (tab: TermTab) => {
    const title = renameVal.trim();
    setRenaming("");
    if (title && title !== tab.title) {
      ws.send({ type: "term_rename", client_id: tab.client_id, term_id: tab.term_id, title });
    }
  };

  // ---- 批量管理 ----
  const toggleBatch = (key: string) => {
    const next = new Set(batchSel);
    if (next.has(key)) next.delete(key); else next.add(key);
    setBatchSel(next);
  };
  // 所有存活终端（按客户端分组），key = clientID/termID
  const aliveGroups = clients
    .map((c) => ({ client: c, terms: Object.values(c.terminals || {}).filter((t) => !t.closed) }))
    .filter((g) => g.terms.length > 0);
  const allAliveKeys = aliveGroups.flatMap((g) => g.terms.map((t) => `${g.client.id}/${t.id}`));
  const selCount = allAliveKeys.filter((k) => batchSel.has(k)).length;

  const batchDetach = () => {
    // 仅从页面移除（终端保留）
    const drop = new Set([...batchSel].filter((k) => tabKeys.has(k)));
    if (!drop.size) return;
    const tabs = termTabs.filter((t) => !drop.has(t.key));
    set({ termTabs: tabs, activeTab: drop.has(activeTab) ? tabs[tabs.length - 1]?.key || "" : activeTab });
    persistUI();
    setBatchSel(new Set());
    setBatchOpen(false);
  };
  const batchAttach = () => {
    // 接入到页面（未在标签中的才需要 openTerm；term_opened 回包会加标签）
    for (const g of aliveGroups) {
      for (const t of g.terms) {
        const key = `${g.client.id}/${t.id}`;
        if (batchSel.has(key) && !tabKeys.has(key)) openTerm(g.client, t.id, t.title);
      }
    }
    setBatchSel(new Set());
    setBatchOpen(false);
  };
  const batchClose = () => {
    // 关闭终端（销毁）
    for (const k of batchSel) {
      const [cid, tid] = k.split("/");
      ws.send({ type: "term_close", client_id: cid, term_id: tid });
    }
    const tabs = termTabs.filter((t) => !batchSel.has(t.key));
    set({ termTabs: tabs, activeTab: batchSel.has(activeTab) ? tabs[tabs.length - 1]?.key || "" : activeTab });
    persistUI();
    setBatchSel(new Set());
    setBatchOpen(false);
  };

  const active = termTabs.find((t) => t.key === activeTab);

  // 可接入的历史终端（已打开但未在标签中的）
  const tabKeys = new Set(termTabs.map((t) => t.key));
  const attachableCount = clients.filter((c) => hasAttachable(c, tabKeys)).length;

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-stretch border-b border-border">
        {/* 标签区：独立横向滚动，不挤压右侧功能区 */}
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto px-2 pt-1">
        {termTabs.map((tab) => (
          <div
            key={tab.key}
            data-testid="term-tab"
            className={cn(
              "group flex items-center gap-1 whitespace-nowrap rounded-t-md border border-b-0 border-border px-3 py-1.5 text-sm cursor-pointer",
              tab.key === activeTab ? "bg-card" : "bg-muted/50 text-muted-foreground"
            )}
            onClick={() => { set({ activeTab: tab.key }); persistUI(); }}
            onContextMenu={(e) => {
              e.preventDefault();
              setMenu({ x: Math.min(e.clientX, window.innerWidth - 200), y: Math.min(e.clientY, window.innerHeight - 140), tab: tab });
            }}
          >
            <span className={cn("h-2 w-2 shrink-0 rounded-full", tab.closed ? "bg-red-500" : "bg-green-500")} />
            {renaming === tab.key ? (
              <input
                data-testid="term-rename-input"
                autoFocus
                className="w-28 rounded border border-primary bg-input px-1 py-0 text-sm outline-none"
                value={renameVal}
                onChange={(e) => setRenameVal(e.target.value)}
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => {
                  if (e.key === "Enter") commitRename(tab);
                  if (e.key === "Escape") setRenaming("");
                }}
                onBlur={() => commitRename(tab)}
              />
            ) : (
              <span
                className="max-w-[180px] truncate"
                title={t("tabTooltip", { client: tab.client_name, title: tab.title })}
                onDoubleClick={(e) => {
                  e.stopPropagation();
                  setRenaming(tab.key);
                  setRenameVal(tab.title);
                }}
              >
                {tab.client_name}:{tab.title}
              </span>
            )}
            <button
              className="ml-1 rounded p-0.5 opacity-0 group-hover:opacity-100 hover:bg-accent"
              title={t("closeDestroy")}
              onClick={(e) => { e.stopPropagation(); closeTab(tab); }}
            >
              <X size={12} />
            </button>
          </div>
        ))}
        </div>

        {/* 右侧固定功能区：批量管理 + 新建终端下拉 + 移除当前标签 */}
        <div className="relative flex shrink-0 items-center gap-1 border-l border-border px-2">
          {active && !active.closed && (
            <Button size="sm" variant="ghost" onClick={() => detachTab(active)}>
              {t("removeFromPage")}
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            data-testid="batch-btn"
            onClick={() => {
              setBatchSel((prev) => new Set([...prev].filter((k) => allAliveKeys.includes(k)))); // 清掉已关闭的勾选
              setBatchOpen(!batchOpen);
            }}
          >
            <ListChecks size={14} /> {t("batchManage")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            data-testid="new-term-btn"
            onClick={() => { setNewOpen(!newOpen); setExpandClient(""); }}
          >
            <Plus size={14} /> {t("newTerm")} <ChevronDown size={12} className={cn("transition-transform", newOpen && "rotate-180")} />
          </Button>

          {batchOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setBatchOpen(false)} />
              <Card
                data-testid="batch-panel"
                className="absolute right-2 top-full z-50 mt-1 flex max-h-[70vh] w-80 flex-col p-1 text-sm shadow-lg"
              >
                <div className="flex items-center justify-between px-2 py-1 text-xs text-muted-foreground">
                  <span>{t("batchSummary", { total: allAliveKeys.length, sel: selCount })}</span>
                  <button
                    data-testid="batch-all"
                    className="text-primary hover:underline"
                    onClick={() => setBatchSel(selCount === allAliveKeys.length ? new Set() : new Set(allAliveKeys))}
                  >
                    {selCount === allAliveKeys.length && allAliveKeys.length > 0 ? t("clearSel") : t("selectAll")}
                  </button>
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto">
                  {aliveGroups.map(({ client, terms }) => (
                    <div key={client.id}>
                      <div className="px-2 py-1 text-xs font-medium text-muted-foreground truncate">
                        {clientName(client)}{!client.online && t("offline")}
                      </div>
                      {terms.map((term) => {
                        const key = `${client.id}/${term.id}`;
                        const inPage = tabKeys.has(key);
                        return (
                          <label
                            key={key}
                            data-testid="batch-item"
                            className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 hover:bg-accent"
                          >
                            <input
                              type="checkbox"
                              checked={batchSel.has(key)}
                              onChange={() => toggleBatch(key)}
                            />
                            <span className="truncate">{term.title}</span>
                            <span className={cn("ml-auto shrink-0 text-[10px]", inPage ? "text-green-500" : "text-muted-foreground")}>
                              {inPage ? t("inPage") : t("notInPage")}
                            </span>
                          </label>
                        );
                      })}
                    </div>
                  ))}
                  {aliveGroups.length === 0 && (
                    <div className="px-2 py-2 text-xs text-muted-foreground">{t("noAliveTerms")}</div>
                  )}
                </div>
                <div className="mt-1 flex gap-1 border-t border-border p-1">
                  <Button size="sm" variant="outline" data-testid="batch-attach" disabled={!selCount} onClick={batchAttach}>
                    {t("attachToPage")}
                  </Button>
                  <Button size="sm" variant="outline" data-testid="batch-detach" disabled={!selCount} onClick={batchDetach}>
                    {t("removeFromPage")}
                  </Button>
                  <Button size="sm" variant="destructive" data-testid="batch-close" disabled={!selCount} onClick={batchClose}>
                    {t("closeTerm")}
                  </Button>
                </div>
              </Card>
            </>
          )}
          {newOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setNewOpen(false)} />
              <Card
                data-testid="new-term-panel"
                className="absolute right-2 top-full z-50 mt-1 max-h-[70vh] w-72 overflow-y-auto p-1 text-sm shadow-lg"
              >
                <div className="px-2 py-1 text-xs text-muted-foreground">{t("newTermHint")}</div>
                {onlineClients.length === 0 && (
                  <div className="px-2 py-2 text-xs text-muted-foreground">{t("noOnlineClients")}</div>
                )}
                {onlineClients.map((c) => {
                  const expanded = expandClient === c.id;
                  return (
                    <div key={c.id}>
                      <button
                        data-testid="new-term-client"
                        className="flex w-full items-center gap-1 rounded px-2 py-1.5 text-left hover:bg-accent"
                        onClick={() => setExpandClient(expanded ? "" : c.id)}
                      >
                        {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                        <span className="truncate font-medium">{clientName(c)}</span>
                        <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">{c.host.match(/\(([^)]+)\)/)?.[1]}</span>
                      </button>
                      {expanded && (
                        <div className="ml-5 border-l border-border pl-1">
                          <ShellRows client={c} onPick={(p) => { openTerm(c, "", "", p); setNewOpen(false); }} />
                        </div>
                      )}
                    </div>
                  );
                })}
                {attachableCount > 0 && (
                  <>
                    <div className="mt-1 border-t border-border px-2 py-1 text-xs text-muted-foreground">{t("attachExisting")}</div>
                    {clients.filter((c) => hasAttachable(c, tabKeys)).map((c) => (
                      <AttachRows
                        key={c.id}
                        client={c}
                        excludeKeys={tabKeys}
                        onPick={(t) => { openTerm(c, t.id, t.title); setNewOpen(false); }}
                      />
                    ))}
                  </>
                )}
              </Card>
            </>
          )}
        </div>
      </div>
      <div className="flex-1 min-h-0">
        {active ? (
          active.closed ? (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              {t("termClosed")}
              <Button variant="outline" size="sm" className="ml-2" onClick={() => detachTab(active)}>{t("removeTab")}</Button>
            </div>
          ) : (
            <TermView key={active.key} clientID={active.client_id} termID={active.term_id} />
          )
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            {t("emptyHint")}
          </div>
        )}
      </div>

      {/* 标签右键菜单 */}
      {menu && (
        <div
          className="fixed inset-0 z-50"
          onClick={() => setMenu(null)}
          onContextMenu={(e) => { e.preventDefault(); setMenu(null); }}
        >
          <Card
            data-testid="term-tab-menu"
            className="absolute w-48 p-1 text-sm shadow-lg"
            style={{ left: menu.x, top: menu.y }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-2 py-1 text-xs text-muted-foreground truncate">{menu.tab.client_name}:{menu.tab.title}</div>
            <button
              data-testid="menu-rename"
              className="flex w-full items-center rounded px-2 py-1.5 text-left hover:bg-accent"
              onClick={() => { setRenaming(menu.tab.key); setRenameVal(menu.tab.title); setMenu(null); }}
            >
              {t("rename")}
            </button>
            <button
              className="flex w-full items-center rounded px-2 py-1.5 text-left hover:bg-accent"
              onClick={() => { detachTab(menu.tab); setMenu(null); }}
            >
              {t("removeKeep")}
            </button>
            <button
              className="flex w-full items-center rounded px-2 py-1.5 text-left text-red-400 hover:bg-accent disabled:opacity-40"
              disabled={menu.tab.closed}
              onClick={() => { closeTab(menu.tab); setMenu(null); }}
            >
              {t("closeDestroy")}
            </button>
          </Card>
        </div>
      )}
    </div>
  );
}
