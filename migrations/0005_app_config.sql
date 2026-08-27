-- 0005 app_config:大模型(AI)配置权威源 = 数据库,页面可编辑。
-- 代码不再读 PIKS_AI_* 环境变量(config.ApplyAppConfig 从本表合并)。
-- 密钥只存在本库,绝不出现在代码/git/env;页面显示掩码,留空不修改。

CREATE TABLE IF NOT EXISTS app_config (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  label      TEXT NOT NULL DEFAULT '',      -- 配置项中文名(展示用)
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 种子安全默认值(密钥留空 = 未配置,部署时从 lab 引导写入)。
INSERT INTO app_config (key, value, label) VALUES
  ('ai_service_base_url',   'https://api.deepseek.com', 'AI 服务地址'),
  ('ai_api_key',            '',                          'API Key'),
  ('ai_model_extract',      'deepseek-chat',             '抽取模型(便宜档)'),
  ('ai_model_reasoning',    'deepseek-reasoner',         '推理模型(贵档)'),
  ('ai_daily_token_budget', '0',                         '日 token 预算(护栏)')
ON CONFLICT (key) DO NOTHING;
