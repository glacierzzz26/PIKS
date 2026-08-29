import type { Flash } from "../types";

/** 演示数据：结构对齐 raw_documents 表（东财 7x24 快讯投影）。 */
export const MOCK_FLASHES: Flash[] = [
  { id: "fl_0901", time: "2026-08-29 14:57", content: "北向资金今日净买入 62.4 亿元，宁德时代、中际旭创获净买入居前。", source: "东财快讯", important: true, event_id: "evt_20260829_001" },
  { id: "fl_0902", time: "2026-08-29 14:32", content: "碳酸锂期货主力合约涨 4.8%，报 7.82 万元/吨，创近三个月新高。", source: "东财快讯", important: true },
  { id: "fl_0903", time: "2026-08-29 13:58", content: "恒生科技指数午后涨幅扩大至 2.1%，半导体板块领涨。", source: "东财快讯", important: false },
  { id: "fl_0904", time: "2026-08-29 11:32", content: "华泰证券：预计三季度 AI 服务器出货量环比增长 25%，持续推荐光模块与 PCB 龙头。", source: "券商研报", important: false },
  { id: "fl_0905", time: "2026-08-29 11:05", content: "宁德时代与三家车企签署长期供货协议，合计规划产能约 180GWh。", source: "公司公告", important: true, event_id: "evt_20260829_001" },
  { id: "fl_0906", time: "2026-08-29 10:21", content: "市场传闻两家中型券商拟合并重组，双方均未回应。", source: "市场传闻", important: false, event_id: "evt_20260829_005" },
  { id: "fl_0907", time: "2026-08-29 09:45", content: "央行开展 3000 亿元 MLF 操作，中标利率 2.30%，净投放 1500 亿元。", source: "央行公告", important: true, event_id: "evt_20260829_002" },
  { id: "fl_0908", time: "2026-08-29 09:25", content: "三大指数集体高开，算力、PCB 方向涨幅居前；恒宝股份 6 连板。", source: "东财快讯", important: false },
  { id: "fl_0909", time: "2026-08-28 19:02", content: "天齐锂业半年报：营收 182 亿元同比 -12%，归母净利 21.4 亿元同比 -34%。", source: "公司公告", important: true, event_id: "evt_20260829_004" },
  { id: "fl_0910", time: "2026-08-28 16:40", content: "光伏反内卷座谈会结束，未形成书面纪要， participants 称限价机制仍在讨论。", source: "产业媒体", important: false, event_id: "evt_20260828_008" },
  { id: "fl_0911", time: "2026-08-28 15:30", content: "华为发布昇腾 950 系列芯片及 CloudMatrix 3.0 集群，四季度批量交付。", source: "科技媒体", important: true, event_id: "evt_20260829_003" },
  { id: "fl_0912", time: "2026-08-28 09:12", content: "隔夜美股：英伟达涨 4.2% 创历史新高，财报后多家投行上调目标价。", source: "海外市场", important: true, event_id: "evt_20260828_007" },
];

export const FLASH_SOURCES = [
  { key: "", label: "全部来源" },
  { key: "东财快讯", label: "东财快讯" },
  { key: "公司公告", label: "公司公告" },
  { key: "券商研报", label: "券商研报" },
  { key: "海外市场", label: "海外市场" },
  { key: "市场传闻", label: "市场传闻" },
];
