"use client";

import AiServiceForm from "@/components/settings/AiServiceForm";
import OpsCard from "@/components/settings/OpsCard";

/** 设置：大模型分层配置（存 app_config）+ 数据与运维子入口 */
export default function Page() {
  return (
    <div>
      <div className="page-head">
        <div>
          <h1>设置</h1>
          <div className="psub">AI 模型配置，以及底层数据入口</div>
        </div>
      </div>
      <div className="max-w-[720px]">
        <AiServiceForm />
        <OpsCard />
      </div>
    </div>
  );
}
