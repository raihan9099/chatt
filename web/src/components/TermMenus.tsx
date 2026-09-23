import type { ClientInfo, TermInfo } from "@/lib/types";
import { clientName } from "@/lib/utils";
import { TerminalSquare } from "lucide-react";
import { useI18n } from "@/i18n";

const zh = {
  defaultShell: "默认",
  clientDefaultShell: "客户端默认 shell",
  defaultTag: "（默认）",
};
const en: Record<keyof typeof zh, string> = {
  defaultShell: "Default",
  clientDefaultShell: "Client default shell",
  defaultTag: " (default)",
};
const dict = { zh, en };

// 新建终端/接入终端下拉面板的共享行组件（终端页与客户端页共用）。

/** 某客户端的 shell 类型列表（新建终端用）；无上报时仅"默认"。 */
export function ShellRows({ client, testid = "new-term-shell", onPick }: {
  client: ClientInfo;
  testid?: string;
  onPick: (shellPath: string) => void;
}) {
  const { t } = useI18n(dict);
  const shells = client.shells || [];
  const rows = shells.length > 0 ? shells : [{ name: t("defaultShell"), path: "" }];
  return (
    <>
      {rows.map((s, i) => (
        <button
          key={s.path || "default"}
          data-testid={testid}
          className="flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left hover:bg-accent"
          title={s.path || t("clientDefaultShell")}
          onClick={() => onPick(s.path)}
        >
          <TerminalSquare size={12} className="shrink-0 text-muted-foreground" />
          <span className="truncate">{s.name}{i === 0 && shells.length > 0 ? t("defaultTag") : ""}</span>
        </button>
      ))}
    </>
  );
}

/** 某客户端可接入的常驻终端列表（已在 excludeKeys 中的除外）。 */
export function AttachRows({ client, excludeKeys, testid = "new-term-attach", onPick }: {
  client: ClientInfo;
  excludeKeys: Set<string>;
  testid?: string;
  onPick: (term: TermInfo) => void;
}) {
  const rows = Object.values(client.terminals || {})
    .filter((t) => !t.closed && !excludeKeys.has(`${client.id}/${t.id}`));
  return (
    <>
      {rows.map((t) => (
        <button
          key={t.id}
          data-testid={testid}
          className="flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left hover:bg-accent"
          onClick={() => onPick(t)}
        >
          <TerminalSquare size={12} className="shrink-0 text-muted-foreground" />
          <span className="truncate">{clientName(client)}:{t.title}</span>
        </button>
      ))}
    </>
  );
}

/** 某客户端是否还有可接入终端（决定分区标题是否显示）。 */
export function hasAttachable(client: ClientInfo, excludeKeys: Set<string>): boolean {
  return Object.values(client.terminals || {}).some((t) => !t.closed && !excludeKeys.has(`${client.id}/${t.id}`));
}
