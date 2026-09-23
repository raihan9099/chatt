// Markdown 渲染：GFM（表格/删除线/任务列表）+ 深色主题排版。
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function Markdown({ children }: { children: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: (p) => <h1 className="mb-2 mt-3 text-lg font-bold first:mt-0" {...p} />,
        h2: (p) => <h2 className="mb-2 mt-3 text-base font-bold first:mt-0" {...p} />,
        h3: (p) => <h3 className="mb-1.5 mt-2.5 text-sm font-bold first:mt-0" {...p} />,
        h4: (p) => <h4 className="mb-1 mt-2 text-sm font-semibold first:mt-0" {...p} />,
        p: (p) => <p className="mb-2 leading-relaxed last:mb-0" {...p} />,
        ul: (p) => <ul className="mb-2 list-disc space-y-0.5 pl-5 last:mb-0" {...p} />,
        ol: (p) => <ol className="mb-2 list-decimal space-y-0.5 pl-5 last:mb-0" {...p} />,
        li: (p) => <li className="leading-relaxed" {...p} />,
        a: (p) => <a className="text-blue-400 underline underline-offset-2 hover:text-blue-300" target="_blank" rel="noreferrer" {...p} />,
        blockquote: (p) => <blockquote className="mb-2 border-l-2 border-primary/50 pl-3 text-muted-foreground last:mb-0" {...p} />,
        hr: () => <hr className="my-3 border-border" />,
        strong: (p) => <strong className="font-semibold text-foreground" {...p} />,
        table: (p) => (
          <div className="mb-2 overflow-x-auto rounded-md border border-border last:mb-0">
            <table className="w-full border-collapse text-xs" {...p} />
          </div>
        ),
        thead: (p) => <thead className="bg-muted" {...p} />,
        th: (p) => <th className="border-b border-border px-2.5 py-1.5 text-left font-medium" {...p} />,
        td: (p) => <td className="border-b border-border/50 px-2.5 py-1.5" {...p} />,
        pre: (p) => (
          <pre className="mb-2 overflow-x-auto rounded-md border border-border bg-background/80 p-2.5 text-xs leading-relaxed last:mb-0" {...p} />
        ),
        code: ({ className, children, ...p }) => {
          // 代码块（fenced/缩进）由 <pre> 提供外框，这里只处理行内代码样式
          const isBlock = /language-/.test(className || "") || String(children).includes("\n");
          if (isBlock) {
            return (
              <code className={className} {...p}>
                {children}
              </code>
            );
          }
          return (
            <code className="rounded bg-background/80 px-1 py-0.5 text-[0.85em] text-amber-300" {...p}>
              {children}
            </code>
          );
        },
      }}
    >
      {children}
    </ReactMarkdown>
  );
}
