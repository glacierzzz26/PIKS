"use client";

import Field from "@/components/settings/Field";
import type { AiFormVals } from "@/components/settings/useAiSettingsForm";
import type { SettingsForm } from "@/lib/types";

type Props = {
  data: SettingsForm;
  vals: AiFormVals;
  set: (k: keyof AiFormVals) => (e: React.ChangeEvent<HTMLInputElement>) => void;
};

/**
 * 同花顺自选同步凭据(issue #87)。
 * 与 AI 配置共用一次保存(同一 POST /settings)。
 * 🔴 ths_cred_ui=false 时**不渲染输入框**(仅说明),因服务端会拒写(公网无鉴权 #78);
 *   否则用户填了才被 400 拒绝 = 撒谎的 UI。
 */
export default function ThsCredFields({ data, vals, set }: Props) {
  if (!data.ths_cred_ui) {
    return (
      <>
        <h3 className="mt-6">同花顺自选同步</h3>
        <p className="fnote">
          自选每天自动从同花顺「我的自选」同步。当前部署不允许从页面写入同花顺凭据
          （公网暴露未鉴权期间，防止他人覆盖凭据把你锁在门外）；请在 lab 直接写 app_config 表。
        </p>
      </>
    );
  }
  return (
    <>
      <h3 className="mt-6">同花顺自选同步</h3>
      <p className="fnote">
        服务器每天自动从这里拉「我的自选」(早/午/晚各一次)。账号密码用于自动登录；也可只填 Cookie(7 天需换一次)。
      </p>

      <Field label="同花顺账号" hint={data.ths_account_masked ? `已配置：${data.ths_account_masked}（留空则不改）` : "手机号或用户名"}>
        <input value={vals.ths_account} onChange={set("ths_account")} placeholder={data.ths_account_masked ? "留空保持原值" : "手机号 / 用户名"} />
      </Field>

      <Field label="同花顺密码" hint={data.ths_password_set ? "已配置（留空则不改）" : "尚未配置"}>
        <input
          type="password"
          value={vals.ths_password}
          onChange={set("ths_password")}
          placeholder={data.ths_password_set ? "留空保持原密码" : "登录密码"}
        />
      </Field>

      <Field
        label="Cookie（可选，优先于账号密码）"
        hint={data.ths_cookie_masked ? `已配置：${data.ths_cookie_masked}（留空则不改）` : "浏览器登录同花顺后复制整条 Cookie"}
      >
        <input
          type="password"
          value={vals.ths_cookie}
          onChange={set("ths_cookie")}
          placeholder={data.ths_cookie_masked ? "留空保持原 Cookie" : "整条 Cookie 字符串"}
        />
      </Field>
    </>
  );
}
