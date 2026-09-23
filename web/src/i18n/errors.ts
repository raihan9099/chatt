// 服务端错误码 → 双语文案。
// 服务端在原有中文 error 文本之外附带稳定 code（HTTP: {"error":..,"code":..,"params":..}；
// WS: {"type":"error","error":..,"code":..}）。前端优先按 code 本地化，未知 code 回退原始文本。
import type { Lang } from "@/i18n";

type Entry = { zh: string; en: string };

export const errorTexts: Record<string, Entry> = {
  // 通用 / 认证
  bad_request: { zh: "请求格式错误", en: "Invalid request" },
  unauthorized: { zh: "未认证", en: "Not authenticated" },
  login_failed: { zh: "登录失败", en: "Login failed" },
  captcha_invalid: { zh: "验证码错误或已过期", en: "Incorrect or expired captcha" },
  bad_credentials: { zh: "用户名或密码错误", en: "Incorrect username or password" },
  // 客户端 / 终端
  client_not_found: { zh: "客户端不存在", en: "Client not found" },
  client_offline: { zh: "客户端不在线", en: "Client offline" },
  client_online_cannot_delete: { zh: "客户端在线，不能删除", en: "Client is online and cannot be deleted" },
  term_not_found: { zh: "终端不存在: {id}", en: "Terminal not found: {id}" },
  term_not_found_or_closed: { zh: "终端不存在或已关闭", en: "Terminal not found or closed" },
  term_closed: { zh: "终端已关闭", en: "Terminal already closed" },
  term_closed_by_user: { zh: "用户关闭", en: "Closed by user" },
  term_closed_not_reported: { zh: "agent 重连后未上报", en: "Not reported after agent reconnect" },
  shell_not_allowed: { zh: "shell 不在可用列表: {shell}", en: "Shell not in the available list: {shell}" },
  holder_timeout: { zh: "等待 holder 就绪超时", en: "Timed out waiting for terminal holder" },
  agent_timeout: { zh: "等待客户端应答超时", en: "Timed out waiting for agent response" },
  // 文件
  file_too_large: { zh: "文件超过 {max}MB 上限", en: "File exceeds the {max}MB limit" },
  bad_base64: { zh: "base64 解码失败", en: "Base64 decode failed" },
  unknown_file_op: { zh: "未知文件操作", en: "Unknown file operation" },
  // AI
  bad_default_mode: { zh: "default_mode 只能是 auto 或 approval", en: "default_mode must be auto or approval" },
  ai_not_configured: { zh: "AI 未配置完整（base_url/api_key/model）", en: "AI not fully configured (base_url/api_key/model)" },
  ai_empty_response: { zh: "模型无响应内容", en: "Empty model response" },
};

/** 按 code 取本地化文本；无 code 或未收录时回退 fallback（服务端原文）。 */
export function errText(
  code: string | undefined,
  fallback: string | undefined,
  lang: Lang,
  params?: Record<string, string | number>
): string {
  const e = code ? errorTexts[code] : undefined;
  if (!e) return fallback || code || "";
  let s = e[lang];
  if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

/** 从 api.ts 抛出的 Error（携带 code/message/params）或 WS error 消息对象取本地化文本。 */
export function errOf(e: any, lang: Lang): string {
  return errText(e?.code, e?.message || (e ? String(e) : ""), lang, e?.params);
}
