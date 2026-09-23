"use client";

import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { apiLogin } from "@/lib/api";

/**
 * 登录页 `/login`（P12 访问控制,issue #78）—— 无壳路由,位于 AppShell 之外。
 * 单密码登录:明文只在本地输入框 → POST /auth/login;成功后跳回 `?next`(或首页)。
 * 视觉沿用全站设计系统(globals.css 的 .login-* + 品牌渐变),暗色自动生效。
 */
export default function Login() {
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [params] = useSearchParams();
  const nav = useNavigate();

  // ?next= 来路;仅接受站内路径(防开放重定向:必须以单个 / 开头,不能是 // 或绝对 URL)。
  const raw = params.get("next") || "/";
  const next = raw.startsWith("/") && !raw.startsWith("//") ? raw : "/";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!password || busy) return;
    setBusy(true);
    setErr(null);
    try {
      await apiLogin(password);
      nav(next, { replace: true });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "登录失败");
      setBusy(false);
    }
  }

  return (
    <div className="login-wrap">
      <div className="login-card">
        <div className="login-brand">
          <span className="login-logo" aria-hidden="true">
            <svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="#fff"
              strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 3v18h18" />
              <path d="M7 14l4-4 3 3 5-6" />
            </svg>
          </span>
          <span className="bt">
            <b>PIKS</b>
            <span>个人投研知识系统</span>
          </span>
        </div>

        <h1>登录</h1>
        <p className="lsub">这是你的私人投研库,先登录再进。会话活跃期间保持登录,闲置 60 分钟后需重新登录。</p>

        {err && <div className="login-err">{err}</div>}

        <form onSubmit={submit}>
          <div className="frow">
            <label htmlFor="pw">密码</label>
            <input
              id="pw"
              type="password"
              autoFocus
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="请输入密码"
            />
          </div>
          <button className="login-btn" type="submit" disabled={busy || !password}>
            {busy ? "登录中…" : "进入"}
          </button>
        </form>

        <div className="login-foot">
          数据只在你自己的库里 · 不预测涨跌、不给买卖建议
        </div>
      </div>
    </div>
  );
}
