import { useEffect } from "react";
import { HashRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { clearToken, getToken, ws } from "@/lib/api";
import { initWSHandlers, useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import Login from "@/pages/Login";
import Clients from "@/pages/Clients";
import Terminals from "@/pages/Terminals";
import Files from "@/pages/Files";
import AI from "@/pages/AI";
import { Bot, FileText, LogOut, Monitor, Server } from "lucide-react";
import { LangSwitch, useI18n } from "@/i18n";

const zh = { clients: "客户端", terminals: "终端", files: "文件", ai: "AI Agent", connected: "已连接", disconnected: "连接断开", logout: "退出" };
const en: Record<keyof typeof zh, string> = {
  clients: "Clients", terminals: "Terminals", files: "Files", ai: "AI Agent", connected: "Connected", disconnected: "Disconnected", logout: "Logout",
};
const appDict = { zh, en };

function Shell() {
  const { page, connected, set, persistUI, hydrated } = useStore();
  const { t } = useI18n(appDict);
  const nav = useNavigate();
  const loc = useLocation();
  // 高亮以实际路由为准，避免与持久化的 page 状态不同步
  const current = loc.pathname.replace(/^\//, "") || page;

  useEffect(() => {
    initWSHandlers();
    ws.connect();
  }, []);

  // 水合完成后按持久化的 page 跳转一次
  useEffect(() => {
    if (hydrated) {
      nav("/" + page, { replace: true });
    }
  }, [hydrated]);

  // 反向同步：直接改地址栏/浏览器前进后退时，把实际路由写回持久化状态
  useEffect(() => {
    const p = loc.pathname.replace(/^\//, "");
    if (p && p !== page && ["clients", "terminals", "files", "ai"].includes(p)) {
      set({ page: p });
      persistUI();
    }
  }, [loc.pathname]);

  const goto = (p: string) => {
    set({ page: p });
    persistUI();
    nav("/" + p);
  };

  const logout = () => {
    ws.close();
    clearToken();
    location.hash = "#/login";
    location.reload();
  };

  const items = [
    { key: "clients", label: t("clients"), icon: Server },
    { key: "terminals", label: t("terminals"), icon: Monitor },
    { key: "files", label: t("files"), icon: FileText },
    { key: "ai", label: t("ai"), icon: Bot },
  ];

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-4 border-b border-border px-4 h-12 shrink-0">
        <span className="font-bold">Termirai</span>
        <nav className="flex gap-1">
          {items.map((it) => (
            <button
              key={it.key}
              onClick={() => goto(it.key)}
              className={cn(
                "flex items-center gap-1 rounded-md px-3 py-1.5 text-sm hover:bg-accent",
                current === it.key && "bg-accent font-medium"
              )}
            >
              <it.icon size={15} /> {it.label}
            </button>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <span className={cn("h-2 w-2 rounded-full", connected ? "bg-green-500" : "bg-red-500")} title={connected ? t("connected") : t("disconnected")} />
          <LangSwitch />
          <button className="text-sm text-muted-foreground hover:text-foreground flex items-center gap-1" onClick={logout}>
            <LogOut size={14} /> {t("logout")}
          </button>
        </div>
      </header>
      <main className="flex-1 min-h-0">
        <Routes>
          <Route path="/clients" element={<Clients />} />
          <Route path="/terminals" element={<Terminals />} />
          <Route path="/files" element={<Files />} />
          <Route path="/ai" element={<AI />} />
          <Route path="*" element={<Navigate to="/clients" replace />} />
        </Routes>
      </main>
    </div>
  );
}

export default function App() {
  const authed = !!getToken();
  return (
    <HashRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/*" element={authed ? <Shell /> : <Navigate to="/login" replace />} />
      </Routes>
    </HashRouter>
  );
}
