// 轻量 i18n：无第三方依赖。每个模块自带 zh/en 双语字典（组件内或同名 *.i18n.ts），
// useI18n(dict) 返回当前语言的 t(key, params)；key 类型由字典 zh 表推导，漏译/拼错无法通过编译。
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type Lang = "zh" | "en";

const LANG_KEY = "termirai_lang";

function detectLang(): Lang {
  try {
    const saved = localStorage.getItem(LANG_KEY);
    if (saved === "zh" || saved === "en") return saved;
  } catch {}
  return (navigator.language || "zh").toLowerCase().startsWith("zh") ? "zh" : "en";
}

const LangCtx = createContext<{ lang: Lang; setLang: (l: Lang) => void }>({ lang: "zh", setLang: () => {} });

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(detectLang);
  useEffect(() => {
    try {
      localStorage.setItem(LANG_KEY, lang);
    } catch {}
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  }, [lang]);
  return <LangCtx.Provider value={{ lang, setLang }}>{children}</LangCtx.Provider>;
}

/** 仅需读写语言（不需要文案）时使用，如语言切换按钮。 */
export function useLang() {
  return useContext(LangCtx);
}

/**
 * 绑定模块字典的翻译器。
 * 字典写法（en 表按 zh 表的 key 强制对齐，漏译编译报错）：
 *   const zh = { hello: "你好" };
 *   const en: Record<keyof typeof zh, string> = { hello: "Hello" };
 *   const dict = { zh, en };
 * 插值：t("hi", { name }) 对应 "你好 {name}" / "Hi {name}"。
 */
export function useI18n<Z extends Record<string, string>>(dict: { zh: Z; en: Record<keyof Z, string> }) {
  const { lang, setLang } = useLang();
  const t = useCallback(
    (key: keyof Z & string, params?: Record<string, string | number>): string => {
      const table = (lang === "en" ? dict.en : dict.zh) as Record<string, string>;
      let s = table[key] ?? dict.zh[key] ?? key;
      if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
      return s;
    },
    [lang, dict]
  );
  return { lang, setLang, t };
}

/** 中/EN 切换按钮（顶栏与登录页共用）。 */
export function LangSwitch({ className }: { className?: string }) {
  const { lang, setLang } = useLang();
  return (
    <button
      className={cn(
        "rounded border border-border px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:border-muted-foreground/40 hover:text-foreground",
        className
      )}
      onClick={() => setLang(lang === "zh" ? "en" : "zh")}
      title={lang === "zh" ? "Switch to English" : "切换为中文"}
    >
      {lang === "zh" ? "EN" : "中文"}
    </button>
  );
}
