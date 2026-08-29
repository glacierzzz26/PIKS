import type { Entity, Relationship } from "../types";

/** 演示数据：结构对齐 entities / relationships 表。 */
export const MOCK_ENTITIES: Entity[] = [
  { id: "ent_catl", type: "company", name: "宁德时代", aliases: ["CATL", "300750"], description: "全球动力电池龙头，市占率约 37%。", status: "active", updated_at: "2026-08-29" },
  { id: "ent_byd", type: "company", name: "比亚迪", aliases: ["002594", "BYD"], description: "新能源整车龙头，垂直整合度高。", status: "active", updated_at: "2026-08-26" },
  { id: "ent_huawei", type: "company", name: "华为", aliases: ["HUAWEI"], description: "ICT 巨头，昇腾算力与鸿蒙生态核心。", status: "active", updated_at: "2026-08-28" },
  { id: "ent_tianqi", type: "company", name: "天齐锂业", aliases: ["002466"], description: "锂矿采选与锂盐加工龙头之一。", status: "watch", updated_at: "2026-08-28" },
  { id: "ent_longi", type: "company", name: "隆基绿能", aliases: ["601012"], description: "光伏硅片与组件双龙头。", status: "active", updated_at: "2026-08-27" },
  { id: "ent_smic", type: "company", name: "中芯国际", aliases: ["688981", "SMIC"], description: "大陆晶圆代工龙头。", status: "active", updated_at: "2026-08-27" },
  { id: "ent_nauro", type: "company", name: "北方华创", aliases: ["002371"], description: "半导体设备平台型公司。", status: "active", updated_at: "2026-08-25" },
  { id: "ent_nvidia", type: "company", name: "英伟达", aliases: ["NVDA"], description: "全球 AI 算力芯片霸主。", status: "active", updated_at: "2026-08-28" },
  { id: "ent_battery", type: "industry", name: "动力电池", aliases: [], description: "新能源汽车核心零部件。", status: "active", updated_at: "2026-08-29" },
  { id: "ent_lithium", type: "industry", name: "锂矿", aliases: ["锂资源"], description: "上游锂资源采选。", status: "active", updated_at: "2026-08-28" },
  { id: "ent_solar", type: "industry", name: "光伏", aliases: ["太阳能"], description: "硅料-硅片-电池-组件产业链。", status: "active", updated_at: "2026-08-27" },
  { id: "ent_semi", type: "industry", name: "半导体", aliases: ["芯片"], description: "设计-制造-封测-设备材料。", status: "active", updated_at: "2026-08-27" },
  { id: "ent_compute", type: "concept", name: "算力", aliases: ["AI算力", "AIDC"], description: "AI 训练与推理基础设施。", status: "active", updated_at: "2026-08-28" },
  { id: "ent_storage", type: "concept", name: "储能", aliases: ["新型储能"], description: "电化学储能与电网侧储能。", status: "active", updated_at: "2026-08-26" },
  { id: "ent_bank", type: "industry", name: "银行", aliases: [], description: "申万一级行业-银行。", status: "active", updated_at: "2026-08-29" },
  { id: "ent_machinery", type: "industry", name: "机械设备", aliases: [], description: "通用与专用设备制造。", status: "watch", updated_at: "2026-08-27" },
  { id: "ent_yuyan", type: "person", name: "曾毓群", aliases: ["Robin Zeng"], description: "宁德时代董事长。", status: "active", updated_at: "2026-08-20" },
  { id: "ent_wangchuanfu", type: "person", name: "王传福", aliases: [], description: "比亚迪董事长。", status: "active", updated_at: "2026-08-20" },
];

export const MOCK_RELATIONSHIPS: Relationship[] = [
  { id: "rel_1", from_id: "ent_yuyan", to_id: "ent_catl", rel_type: "高管", confidence: 0.99 },
  { id: "rel_2", from_id: "ent_wangchuanfu", to_id: "ent_byd", rel_type: "高管", confidence: 0.99 },
  { id: "rel_3", from_id: "ent_catl", to_id: "ent_battery", rel_type: "属于行业", confidence: 0.98 },
  { id: "rel_4", from_id: "ent_byd", to_id: "ent_battery", rel_type: "采购方", confidence: 0.92 },
  { id: "rel_5", from_id: "ent_battery", to_id: "ent_lithium", rel_type: "上游", confidence: 0.9 },
  { id: "rel_6", from_id: "ent_tianqi", to_id: "ent_lithium", rel_type: "属于行业", confidence: 0.98 },
  { id: "rel_7", from_id: "ent_catl", to_id: "ent_tianqi", rel_type: "供应链", confidence: 0.81 },
  { id: "rel_8", from_id: "ent_huawei", to_id: "ent_compute", rel_type: "属于概念", confidence: 0.93 },
  { id: "rel_9", from_id: "ent_nvidia", to_id: "ent_compute", rel_type: "属于概念", confidence: 0.97 },
  { id: "rel_10", from_id: "ent_smic", to_id: "ent_semi", rel_type: "属于行业", confidence: 0.98 },
  { id: "rel_11", from_id: "ent_nauro", to_id: "ent_semi", rel_type: "属于行业", confidence: 0.97 },
  { id: "rel_12", from_id: "ent_smic", to_id: "ent_nauro", rel_type: "客户", confidence: 0.86 },
  { id: "rel_13", from_id: "ent_longi", to_id: "ent_solar", rel_type: "属于行业", confidence: 0.98 },
  { id: "rel_14", from_id: "ent_byd", to_id: "ent_storage", rel_type: "属于概念", confidence: 0.84 },
  { id: "rel_15", from_id: "ent_nvidia", to_id: "ent_smic", rel_type: "竞争/替代", confidence: 0.55 },
];

export const ENTITY_TYPES = [
  { key: "", label: "全部" },
  { key: "company", label: "公司" },
  { key: "industry", label: "行业" },
  { key: "concept", label: "概念" },
  { key: "person", label: "人物" },
];
