import { useCallback, useEffect, useRef, useState } from "react";
import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import { rest } from "@/lib/api";
import { useStore } from "@/lib/store";
import { b64decode, clientName } from "@/lib/utils";
import { Button } from "@/components/ui";
import { ArrowLeft, FilePlus, FolderPlus, Pencil, RefreshCw, Save, Trash2 } from "lucide-react";
import type { FileEntry } from "@/lib/types";
import { useI18n } from "@/i18n";
import { errOf } from "@/i18n/errors";

const zh = {
  title: "文件管理",
  parentDir: "上级目录",
  refresh: "刷新",
  newFile: "文件",
  newDir: "目录",
  save: "保存",
  rename: "重命名",
  delete: "删除",
  emptyDir: "（空目录）",
  pickFileHint: "选择左侧文件进行编辑（文本文件直接在内嵌 Monaco 编辑器中打开）",
  noOnlineClients: "暂无在线客户端。请在目标机器上运行 termirai-agent。",
  newFileName: "新文件名：",
  newDirName: "新目录名：",
  newName: "新名称：",
  confirmDelete: "删除 {path}？",
};
const en: Record<keyof typeof zh, string> = {
  title: "File manager",
  parentDir: "Parent directory",
  refresh: "Refresh",
  newFile: "File",
  newDir: "Folder",
  save: "Save",
  rename: "Rename",
  delete: "Delete",
  emptyDir: "(Empty)",
  pickFileHint: "Select a file on the left to edit (text files open directly in the embedded Monaco editor)",
  noOnlineClients: "No clients online. Run termirai-agent on the target machine first.",
  newFileName: "New file name:",
  newDirName: "New folder name:",
  newName: "New name:",
  confirmDelete: "Delete {path}?",
};
const dict = { zh, en };

self.MonacoEnvironment = { getWorker: () => new editorWorker() };
loader.config({ monaco });

function langOf(path: string): string {
  const ext = path.split(".").pop()?.toLowerCase() || "";
  const map: Record<string, string> = {
    ts: "typescript", tsx: "typescript", js: "javascript", jsx: "javascript", json: "json",
    go: "go", py: "python", sh: "shell", bash: "shell", yml: "yaml", yaml: "yaml",
    md: "markdown", html: "html", css: "css", xml: "xml", sql: "sql", conf: "ini", ini: "ini",
    toml: "ini", rs: "rust", c: "c", h: "c", cpp: "cpp", java: "java", txt: "plaintext",
  };
  return map[ext] || "plaintext";
}

export default function Files() {
  const { clients, filesState, set, persistUI } = useStore();
  const { lang, t } = useI18n(dict);
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [err, setErr] = useState("");
  const [renaming, setRenaming] = useState<FileEntry | null>(null);
  const editorRef = useRef<any>(null);

  const clientID = filesState?.client_id || "";
  const path = filesState?.path || "/";
  const openFile = filesState?.open_file;
  const client = clients.find((c) => c.id === clientID);

  const setFS = (p: Partial<NonNullable<typeof filesState>>) => {
    set({ filesState: { client_id: clientID, path, open_file: openFile, ...p } });
    persistUI();
  };

  const refresh = useCallback(async () => {
    if (!clientID) return;
    try {
      setEntries(await rest.fileList(clientID, path));
      setErr("");
    } catch (e: any) {
      setErr(errOf(e, lang));
      setEntries([]);
    }
  }, [clientID, path, lang]);

  useEffect(() => { refresh(); }, [refresh]);

  const openEntry = async (e: FileEntry) => {
    if (e.dir) {
      setFS({ path: e.path, open_file: undefined });
      setFileContent(null);
    } else {
      try {
        const b64 = await rest.fileRead(clientID, e.path);
        setFileContent(b64decode(b64));
        setFS({ open_file: e.path });
        setDirty(false);
        setErr("");
      } catch (err: any) {
        setErr(errOf(err, lang));
      }
    }
  };

  // 页面状态恢复：有水合的 open_file 时自动打开
  useEffect(() => {
    if (openFile && fileContent === null) {
      rest.fileRead(clientID, openFile).then((b64) => setFileContent(b64decode(b64))).catch(() => {});
    }
  }, [openFile]);

  const save = async () => {
    if (!openFile || fileContent === null) return;
    try {
      await rest.fileWrite(clientID, openFile, fileContent);
      setDirty(false);
      setErr("");
    } catch (e: any) {
      setErr(errOf(e, lang));
    }
  };

  const goUp = () => {
    const parts = path.split("/").filter(Boolean);
    parts.pop();
    setFS({ path: "/" + parts.join("/"), open_file: undefined });
    setFileContent(null);
  };

  const createFile = async () => {
    const name = prompt(t("newFileName"));
    if (!name) return;
    const p = (path === "/" ? "" : path) + "/" + name;
    try {
      await rest.fileWrite(clientID, p, "");
      refresh();
    } catch (e: any) { setErr(errOf(e, lang)); }
  };

  const createDir = async () => {
    const name = prompt(t("newDirName"));
    if (!name) return;
    const p = (path === "/" ? "" : path) + "/" + name;
    try {
      await rest.fileMkdir(clientID, p);
      refresh();
    } catch (e: any) { setErr(errOf(e, lang)); }
  };

  const remove = async (e: FileEntry) => {
    if (!confirm(t("confirmDelete", { path: e.path }))) return;
    try {
      await rest.fileDelete(clientID, e.path);
      if (openFile === e.path) { setFS({ open_file: undefined }); setFileContent(null); }
      refresh();
    } catch (err: any) { setErr(errOf(err, lang)); }
  };

  const rename = async () => {
    if (!renaming) return;
    const name = prompt(t("newName"), renaming.name);
    if (!name || name === renaming.name) { setRenaming(null); return; }
    const dir = renaming.path.slice(0, renaming.path.length - renaming.name.length);
    try {
      await rest.fileRename(clientID, renaming.path, dir + name);
      setRenaming(null);
      refresh();
    } catch (e: any) { setErr(errOf(e, lang)); setRenaming(null); }
  };
  useEffect(() => { if (renaming) rename(); }, [renaming]);

  if (!clientID) {
    const online = clients.filter((c) => c.online);
    return (
      <div className="p-4 space-y-3">
        <h2 className="text-lg font-semibold">{t("title")}</h2>
        <div className="flex gap-2 flex-wrap">
          {online.map((c) => (
            <Button key={c.id} variant="outline" onClick={() => { set({ filesState: { client_id: c.id, path: "/" } }); persistUI(); }}>
              {clientName(c)}
            </Button>
          ))}
        </div>
        {online.length === 0 && <div className="text-sm text-muted-foreground">{t("noOnlineClients")}</div>}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col p-3 gap-2">
      <div className="flex items-center gap-2 text-sm">
        <Button size="sm" variant="outline" onClick={() => { set({ filesState: null }); persistUI(); }}>
          {clientName(client) || clientID}
        </Button>
        <Button size="icon" variant="ghost" onClick={goUp} title={t("parentDir")}><ArrowLeft size={16} /></Button>
        <code className="rounded bg-muted px-2 py-1 flex-1 truncate">{path}</code>
        <Button size="icon" variant="ghost" onClick={refresh} title={t("refresh")}><RefreshCw size={16} /></Button>
        <Button data-testid="btn-new-file" size="sm" variant="outline" onClick={createFile}><FilePlus size={14} /> {t("newFile")}</Button>
        <Button size="sm" variant="outline" onClick={createDir}><FolderPlus size={14} /> {t("newDir")}</Button>
        {openFile && (
          <Button data-testid="btn-save-file" size="sm" onClick={save} disabled={!dirty}><Save size={14} /> {t("save")}{dirty ? " *" : ""}</Button>
        )}
      </div>
      {err && <div className="text-sm text-red-500">{err}</div>}
      <div className="flex flex-1 min-h-0 gap-2">
        <div data-testid="file-list" className="w-72 shrink-0 overflow-auto rounded-md border border-border">
          {entries.map((e) => (
            <div
              key={e.path}
              data-testid="file-row"
              data-name={e.name}
              className="group flex items-center gap-1 px-2 py-1 text-sm cursor-pointer hover:bg-accent"
              onClick={() => openEntry(e)}
            >
              <span>{e.dir ? "📁" : "📄"}</span>
              <span className="flex-1 truncate">{e.name}</span>
              <span className="hidden group-hover:flex gap-1">
                <button title={t("rename")} onClick={(ev) => { ev.stopPropagation(); setRenaming(e); }}><Pencil size={13} /></button>
                <button title={t("delete")} onClick={(ev) => { ev.stopPropagation(); remove(e); }}><Trash2 size={13} /></button>
              </span>
            </div>
          ))}
          {entries.length === 0 && <div className="p-3 text-sm text-muted-foreground">{t("emptyDir")}</div>}
        </div>
        <div className="flex-1 min-w-0 rounded-md border border-border overflow-hidden">
          {openFile && fileContent !== null ? (
            <Editor
              height="100%"
              language={langOf(openFile)}
              value={fileContent}
              theme="vs-dark"
              onChange={(v) => { setFileContent(v ?? ""); setDirty(true); }}
              onMount={(ed) => (editorRef.current = ed)}
              options={{ fontSize: 13, minimap: { enabled: false }, automaticLayout: true }}
            />
          ) : (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t("pickFileHint")}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
