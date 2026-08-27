-- 迭代 5-3 前置:截图/视觉模型配置键(app_config,settings 页下拉选择)。
-- 空 = 5-3 截图回退文本模型(如实降级);lab 上由用户经 /settings 选 Zen 视觉模型(如 deepseek-v4-flash-vision-exp)。
INSERT INTO app_config (key, value, label, updated_at) VALUES
  ('ai_model_vision', '', '截图/视觉模型(5-3 截图识别;空=回退文本模型)', now())
ON CONFLICT (key) DO NOTHING;
