/** 演示数据：交易 / 持仓 / 复盘诊断 / AI 对话 / 设置（只读投影） */

export type TradeRow = {
  date: string;
  code: string;
  name: string;
  side: "buy" | "sell";
  price: number;
  qty: number;
  amount: number;
  source: "manual" | "screenshot";
  note?: string;
};

export const TRADES: TradeRow[] = [
  { date: "2026-08-29", code: "002371", name: "北方华创", side: "buy", price: 412.6, qty: 200, amount: 82520, source: "screenshot", note: "大基金三期事件驱动" },
  { date: "2026-08-28", code: "300476", name: "胜宏科技", side: "buy", price: 58.2, qty: 1200, amount: 69840, source: "screenshot" },
  { date: "2026-08-27", code: "600519", name: "某消费电子", side: "sell", price: 23.4, qty: 3000, amount: 70200, source: "manual", note: "逻辑证伪,清仓" },
  { date: "2026-08-26", code: "002466", name: "天齐锂业", side: "buy", price: 36.8, qty: 1500, amount: 55200, source: "screenshot" },
  { date: "2026-08-25", code: "300750", name: "宁德时代", side: "buy", price: 268.4, qty: 300, amount: 80520, source: "screenshot" },
];

export type PositionRow = {
  code: string;
  name: string;
  qty: number;
  cost: number;
  last: number;
  pnl_pct: number;
};

export const POSITIONS: PositionRow[] = [
  { code: "002371", name: "北方华创", qty: 200, cost: 412.6, last: 442.0, pnl_pct: 7.12 },
  { code: "300750", name: "宁德时代", qty: 300, cost: 268.4, last: 281.6, pnl_pct: 4.92 },
  { code: "300476", name: "胜宏科技", qty: 1200, cost: 58.2, last: 60.4, pnl_pct: 3.78 },
  { code: "002466", name: "天齐锂业", qty: 1500, cost: 36.8, last: 35.1, pnl_pct: -4.62 },
];

export type ReviewRow = {
  date: string;
  scope: string;
  summary: string;
  refs: number;
  state: "positive" | "negative" | "neutral";
};

export const REVIEWS: ReviewRow[] = [
  { date: "2026-08-29", scope: "北方华创 · 事件驱动加仓", summary: "加仓逻辑与大基金三期事件吻合,买入时点在消息确认后 2 小时,未抢跑;当前浮盈 7.1%,持有条件为订单验证落地。", refs: 4, state: "positive" },
  { date: "2026-08-27", scope: "消费电子清仓", summary: "清仓决策正确:砍单数据 8-26 确认,卖出价高于利空前收盘。失误在于 8-19 首次出现砍单传闻时未减仓,纪律执行滞后 6 个交易日。", refs: 6, state: "neutral" },
  { date: "2026-08-25", scope: "天齐锂业建仓", summary: "建仓依据为期货涨停映射,属情绪传导逻辑,持续性存疑;建议以 5 日线为持有底线。", refs: 3, state: "negative" },
];

export type ChatMsg = {
  role: "user" | "assistant";
  content: string;
  time: string;
  refs?: string[];
};

export const CHAT_HISTORY: ChatMsg[] = [
  { role: "user", content: "北方华创这次加仓的逻辑链是什么?帮我对照知识库检查有没有漏掉的风险点。", time: "14:02" },
  { role: "assistant", content: "你 8-29 加仓北方华创的逻辑链:① 大基金三期 8-27 首批投资 220 亿(置信度 0.92);② 半导体设备国产化率目标上调;③ 公司中报订单同比 +38%。\n\n风险点提示:1) 事件源为产业媒体转述,尚未见公司公告确认;2) 检索到 8-21 你自己的错题笔记「追高事件落地后第二天」,当前已是事件后第 2 个交易日;3) 北向资金近 3 日对设备板块净卖出。", time: "14:02", refs: ["事件 #evt_0827_003", "信念:事件驱动买入时点", "错题:追高事件落地", "观测:北向资金"] },
  { role: "user", content: "本周情绪周期走到哪个阶段了?", time: "11:20" },
  { role: "assistant", content: "按你的信念框架(梯队高度=温度计):本周最高板 4→5→6 递进,炸板率 31%→26%,情绪分 41→71,处于**发酵期**。对照框架:最高板 ≥6 且炸板率 <30%,可参与主线中军,但今日炸板率已逼近 30% 阈值,分歧风险在积累。", time: "11:20", refs: ["信念:涨停梯队高度与情绪周期", "市场快照 08-29"] },
];

export const SETTINGS_CFG = [
  { group: "抽取模型(extract)", rows: [["服务地址", "https://opencode.ai/zen/go/v1"], ["模型", "claude-sonnet-4"], ["温度", "0.2"], ["状态", "正常"]] },
  { group: "推理模型(reasoning)", rows: [["服务地址", "https://opencode.ai/zen/go/v1"], ["模型", "gpt-5.2"], ["温度", "0.4"], ["状态", "正常"]] },
  { group: "视觉模型(vision)", rows: [["服务地址", "https://opencode.ai/zen/go/v1"], ["模型", "gemini-3-pro-vision"], ["温度", "0.1"], ["状态", "未验证"]] },
];
