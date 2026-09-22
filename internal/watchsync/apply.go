package watchsync

// apply.go:把「上游快照 → 变更计划」落到 DB(整轮原子)。
//
// 一轮 = 一次 pgx 事务:取名(网络,在事务**外**)→ [建实体 → 置 watch/archived
// → 落价/日](事务内)。任一步失败整轮回滚,幂等可重来(下轮重跑自然收敛)。
//
// 🔴 降级:selfstock_detail 单独失败**不** fail 整轮 —— 名单照常同步,价/日不更新,
//   stats.DetailFailed=true 由 cmd 记进 task_runs.meta(可见,不静默 —— #64 教训)。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"piks/internal/model"
	"piks/internal/store"
	"piks/internal/ths"
)

// Upstream 上游只读接口(生产实现 = *ths.Client;测试可传假实现,零网络)。
type Upstream interface {
	SelfStocks(ctx context.Context) ([]ths.SelfStock, error)
	SelfStockDetails(ctx context.Context) ([]ths.Detail, error)
	StockNames(ctx context.Context, codes []string) (map[string]string, error)
	UserID() string
	CookieSource() string
}

// Stats 一轮同步的结果(cmd 据此写 task_runs.meta)。
type Stats struct {
	Upstream     int       `json:"upstream"` // 上游名单条数(过滤前)
	Kept         int       `json:"kept"`     // 过滤后 A 股条数
	Dropped      []Dropped `json:"dropped"`  // 被过滤条目(带原因)
	Add          int       `json:"add"`
	Keep         int       `json:"keep"`
	Remove       int       `json:"remove"`         // 本轮移出数
	Deferred     []string  `json:"deferred_codes"` // 名称未解析,本轮未建实体(下轮补)
	PriceMissing int       `json:"price_missing"`  // 价缺失条数(常态,非错误)
	DetailFailed bool      `json:"detail_failed"`  // selfstock_detail 失败(降级:价/日未更新)
}

// nameResolution 取名结果:names = 命中的 code→名;deferred = 未解析、本轮不建实体。
type nameResolution struct {
	names    map[string]string
	deferred []string
}

// Apply 跑一轮同步。dryRun=true 时只算计划、不写库(供 -dry-run 与人工核对)。
func Apply(ctx context.Context, s *store.Store, up Upstream, dryRun bool) (Stats, error) {
	var st Stats

	raw, err := up.SelfStocks(ctx)
	if err != nil {
		return st, fmt.Errorf("拉自选名单: %w", err)
	}
	st.Upstream = len(raw)

	kept, dropped := Filter(raw)
	st.Kept, st.Dropped = len(kept), dropped

	// 🔴 反向守卫(#64 教训:不空成功):上游有数据却一只都没通过过滤,说明
	// 过滤规则(白名单/代码校验)或上游结构坏了 —— 必须失败,绝不静默写「0 只」。
	if len(raw) > 0 && len(kept) == 0 {
		return st, fmt.Errorf("上游 %d 条但过滤后 0 条(A 股白名单/代码格式可能已失效)", len(raw))
	}
	// 🔴 上游名单为空 = cookie 失效 / 协议变更(用户不可能真把自选清空)。
	if len(raw) == 0 {
		return st, errors.New("上游自选为空(cookie 可能已失效或协议变更);为防误清空,本轮不改任何状态")
	}

	// 元数据单独失败 → 降级(名单照常,价/日不更新)。绝不因此 fail 整轮。
	var details []ths.Detail
	if ds, derr := up.SelfStockDetails(ctx); derr != nil {
		st.DetailFailed = true
	} else {
		details = ds
	}
	entries, priceMissing := Merge(kept, details)
	st.PriceMissing = priceMissing

	// 现有在选自选(entities.status='watch' 为成员资格真源)。
	watchEnts, err := s.ListEntitiesByStatus(ctx, "watch")
	if err != nil {
		return st, fmt.Errorf("读现有自选: %w", err)
	}
	existing := make([]Existing, 0, len(watchEnts))
	existingByCode := make(map[string]string, len(watchEnts))
	for _, e := range watchEnts {
		code := entityCode(e)
		if code == "" {
			continue
		}
		existing = append(existing, Existing{Code: code, EntityID: e.ID})
		existingByCode[code] = e.ID
	}

	plan := Diff(existing, entries)
	st.Add, st.Keep, st.Remove = plan.Counts()

	if dryRun {
		return st, nil
	}

	// ── 取名(网络;事务外)── 只对「需要新建实体」的 code 取(add 且本地无实体)。
	needName := map[string]bool{}
	needCodes := []string{}
	for _, c := range plan.Changes {
		if c.Kind == ChangeRemove {
			continue
		}
		if _, has := existingByCode[c.Code]; has {
			continue // 已有实体,无需取名
		}
		if !needName[c.Code] {
			needName[c.Code] = true
			needCodes = append(needCodes, c.Code)
		}
	}
	res := resolveNames(ctx, s, up, needCodes)
	st.Deferred = res.deferred

	// ── 落库(整轮原子)──
	tx, err := s.Begin(ctx)
	if err != nil {
		return st, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	applied := Stats{Upstream: st.Upstream, Kept: st.Kept, Dropped: st.Dropped,
		Add: st.Add, Keep: st.Keep, Remove: st.Remove, PriceMissing: st.PriceMissing,
		DetailFailed: st.DetailFailed, Deferred: st.Deferred}

	for _, c := range plan.Changes {
		switch c.Kind {
		case ChangeAdd, ChangeKeep:
			entityID, ok := existingByCode[c.Code]
			if !ok {
				name := res.names[c.Code]
				if name == "" {
					continue // 已进 res.deferred,本轮不建实体(宁缺毋假)
				}
				id, err := store.EnsureCompanyEntityTx(ctx, tx, c.Code, name)
				if err != nil {
					return st, fmt.Errorf("建实体 %s: %w", c.Code, err)
				}
				entityID = id
			}
			if _, err := store.SetEntityStatusTx(ctx, tx, entityID, "watch"); err != nil {
				return st, fmt.Errorf("置 watch %s: %w", c.Code, err)
			}
			if err := store.UpsertWatchlistEntryTx(ctx, tx, c.Code, &entityID,
				c.Entry.Market, c.Entry.MarketID, c.Entry.Price, c.Entry.AddedOn, c.Entry.Raw); err != nil {
				return st, fmt.Errorf("落价/日 %s: %w", c.Code, err)
			}
		case ChangeRemove:
			if c.EntityID == "" {
				continue // 无实体可置 archived(理论上不该发生)
			}
			if _, err := store.SetEntityStatusTx(ctx, tx, c.EntityID, "archived"); err != nil {
				return st, fmt.Errorf("置 archived %s: %w", c.Code, err)
			}
			if _, err := store.MarkWatchlistRemovedTx(ctx, tx, c.Code); err != nil {
				return st, fmt.Errorf("标记移出 %s: %w", c.Code, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return st, err
	}
	return applied, nil
}

// resolveNames 三级回退取名(宁缺毋假):
//
//	① 本地 entities(零外呼)—— entity-build 每日建的股多已覆盖;
//	② 同花顺 realhead(同一上游,无签名);
//	③ 仍缺 → deferred(**不拿 code 当名建实体**,否则污染 (type,name) 唯一键与 ⌘K)。
func resolveNames(ctx context.Context, s *store.Store, up Upstream, codes []string) nameResolution {
	out := nameResolution{names: map[string]string{}}
	if len(codes) == 0 {
		return out
	}
	// ① 本地
	if local, err := s.CompanyNamesByCodes(ctx, codes); err == nil {
		for k, v := range local {
			out.names[k] = v
		}
	}
	var missing []string
	for _, c := range codes {
		if out.names[c] == "" {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		return out
	}
	// ② 同花顺 realhead(单只失败不中断整批;整体错误也只是让它们 deferred)
	if remote, err := up.StockNames(ctx, missing); err == nil {
		for k, v := range remote {
			if v != "" {
				out.names[k] = v
			}
		}
	}
	// ③ 仍缺 → deferred
	for _, c := range missing {
		if out.names[c] == "" {
			out.deferred = append(out.deferred, c)
		}
	}
	return out
}

// entityCode 从实体 detail 取 6 位代码(与 internal/web 的同名函数同口径,
// 但本包**不 import web** —— 故本地小实现,避免把 HTTP/AI 依赖拖进 tools 镜像)。
func entityCode(e model.Entity) string {
	if e.Type != "company" || len(e.Detail) == 0 {
		return ""
	}
	var d struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(e.Detail, &d); err != nil {
		return ""
	}
	return store.NormalizeCode(d.Code)
}
