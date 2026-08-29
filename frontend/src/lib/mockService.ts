/**
 * 演示数据服务：与 REST 端点同形。
 * 后端不可达时由 useData 自动降级到这里，保持界面完整可浏览。
 */
import { MOCK_EVENTS } from "./mock/events";
import { MOCK_ENTITIES, MOCK_RELATIONSHIPS } from "./mock/entities";
import { MOCK_MARKET } from "./mock/market";
import { MOCK_FLASHES } from "./mock/flashes";
import { MOCK_DOCS } from "./mock/docs";
import type {
  EventItem,
  Entity,
  Relationship,
  MarketSnapshot,
  Flash,
  Doc,
} from "./types";

function matchKw(text: string, q?: string): boolean {
  if (!q) return true;
  return text.toLowerCase().includes(q.toLowerCase());
}

export function getEvents(p: {
  type?: string;
  status?: string;
  q?: string;
}): EventItem[] {
  return MOCK_EVENTS.filter(
    (e) =>
      (!p.type || e.event_type === p.type) &&
      (!p.status || e.status === p.status) &&
      (matchKw(e.title, p.q) || matchKw(e.summary, p.q))
  );
}

export function getEntities(p: { type?: string; q?: string }): Entity[] {
  return MOCK_ENTITIES.filter(
    (e) =>
      (!p.type || e.type === p.type) &&
      (matchKw(e.name, p.q) || e.aliases.some((a) => matchKw(a, p.q)))
  );
}

export function getRelationships(): Relationship[] {
  return MOCK_RELATIONSHIPS;
}

export function getMarket(): MarketSnapshot {
  return MOCK_MARKET;
}

export function getFlashes(p: { q?: string; source?: string }): Flash[] {
  return MOCK_FLASHES.filter(
    (f) =>
      (!p.source || f.source === p.source) && matchKw(f.content, p.q)
  );
}

export function getDocs(): Doc[] {
  return MOCK_DOCS;
}

export function getDoc(id: string): Doc | null {
  return MOCK_DOCS.find((d) => d.id === id) ?? null;
}
