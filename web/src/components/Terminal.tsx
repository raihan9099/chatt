// xterm.js 终端组件：订阅服务端输出、发送输入、自适应尺寸。
import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { ws } from "@/lib/api";
import { b64encode } from "@/lib/utils";
import "@xterm/xterm/css/xterm.css";

export function TermView({ clientID, termID }: { clientID: string; termID: string }) {
  const hostRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);

  useEffect(() => {
    const term = new Terminal({
      fontSize: 13,
      fontFamily: "'Cascadia Mono', Consolas, monospace",
      cursorBlink: true,
      theme: { background: "#0c1018", foreground: "#e2e8f0" },
      scrollback: 5000,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current!);
    fit.fit();
    termRef.current = term;

    // 订阅输出（term_sub 会触发服务端补发回放缓冲）
    ws.send({ type: "term_sub", client_id: clientID, term_id: termID });
    ws.send({ type: "term_resize", client_id: clientID, term_id: termID, cols: term.cols, rows: term.rows });

    const unsub = ws.subscribe((msg) => {
      if (msg.type === "term_output" && msg.client_id === clientID && msg.term_id === termID) {
        term.write(Uint8Array.from(atob(msg.data), (c) => c.charCodeAt(0)));
      }
    });
    const disposeData = term.onData((data) => {
      ws.send({ type: "term_input", client_id: clientID, term_id: termID, data: b64encode(data) });
    });
    const disposeResize = term.onResize(({ cols, rows }) => {
      ws.send({ type: "term_resize", client_id: clientID, term_id: termID, cols, rows });
    });
    const ro = new ResizeObserver(() => {
      try { fit.fit(); } catch {}
    });
    ro.observe(hostRef.current!);

    return () => {
      unsub();
      disposeData.dispose();
      disposeResize.dispose();
      ro.disconnect();
      ws.send({ type: "term_unsub", client_id: clientID, term_id: termID });
      term.dispose();
    };
  }, [clientID, termID]);

  return <div ref={hostRef} className="xterm-host h-full w-full bg-[#0d1117] rounded-b-md overflow-hidden" />;
}
