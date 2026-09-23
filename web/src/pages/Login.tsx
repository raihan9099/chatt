import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { rest, setToken } from "@/lib/api";
import { Button, Card, Input } from "@/components/ui";
import { LangSwitch, useI18n } from "@/i18n";
import { errOf } from "@/i18n/errors";

const zh = {
  subtitle: "多机终端管理平台",
  user: "用户名",
  pass: "密码",
  captcha: "验证码",
  clickToRefresh: "点击刷新",
  submit: "登录",
  submitting: "登录中…",
  captchaFailed: "获取验证码失败",
  locked: "登录已锁定，请 {min} 分钟后再试",
};
const en: Record<keyof typeof zh, string> = {
  subtitle: "Multi-host terminal management platform",
  user: "Username",
  pass: "Password",
  captcha: "Captcha",
  clickToRefresh: "Click to refresh",
  submit: "Sign in",
  submitting: "Signing in…",
  captchaFailed: "Failed to load captcha",
  locked: "Login locked, try again in {min} min",
};
const dict = { zh, en };

export default function Login() {
  const nav = useNavigate();
  const { lang, t } = useI18n(dict);
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [captchaID, setCaptchaID] = useState("");
  const [captchaImg, setCaptchaImg] = useState("");
  const [captcha, setCaptcha] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  const refreshCaptcha = useCallback(async () => {
    setCaptcha("");
    try {
      const d = await rest.captcha();
      setCaptchaID(d.captcha_id);
      setCaptchaImg(d.image);
    } catch {
      setErr(t("captchaFailed"));
    }
  }, [t]);

  useEffect(() => {
    refreshCaptcha();
  }, [refreshCaptcha]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setErr("");
    try {
      const d = await rest.login({ user, password: pass, captcha_id: captchaID, captcha });
      setToken(d.token);
      nav("/", { replace: true });
      location.reload(); // 重建 WS
    } catch (e: any) {
      if (e.locked_seconds) {
        setErr(t("locked", { min: Math.ceil(e.locked_seconds / 60) }));
      } else {
        setErr(errOf(e, lang));
      }
      refreshCaptcha();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative flex h-full items-center justify-center bg-muted/40">
      <div className="absolute right-4 top-4">
        <LangSwitch />
      </div>
      <Card className="w-[360px] p-6">
        <h1 className="mb-1 text-xl font-bold">Termirai</h1>
        <p className="mb-4 text-sm text-muted-foreground">{t("subtitle")}</p>
        <form onSubmit={submit} className="space-y-3">
          <Input placeholder={t("user")} value={user} onChange={(e) => setUser(e.target.value)} autoFocus />
          <Input placeholder={t("pass")} type="password" value={pass} onChange={(e) => setPass(e.target.value)} />
          <div className="flex gap-2">
            <Input placeholder={t("captcha")} value={captcha} onChange={(e) => setCaptcha(e.target.value)} className="flex-1" />
            {captchaImg && (
              <img
                src={captchaImg}
                alt={t("captcha")}
                title={t("clickToRefresh")}
                className="h-9 cursor-pointer rounded-md border border-border"
                onClick={refreshCaptcha}
              />
            )}
          </div>
          {err && <div className="text-sm text-red-500">{err}</div>}
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? t("submitting") : t("submit")}
          </Button>
        </form>
      </Card>
    </div>
  );
}
