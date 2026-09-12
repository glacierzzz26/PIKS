package store

// 个股深研归档存取(research 并入,design research-merge.md D-5/D-9;迁移 0012)。
// metrics = 确定性计算(Fact);synthesis = LLM 定性(Opinion);evidence = Fact 溯源链。
// 同 code 多 run = 时间序列(as_of DESC),支撑后续 delta 复研。

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const researchRunCols = `id,run_id,code,symbol,profile,as_of,status,metrics,synthesis,markdown,lint,gate,evidence,error,model,tokens,created_at,updated_at`

// ResearchRun 一次深研快照(status 状态机见设计 §4.4)。
type ResearchRun struct {
	ID        string          `db:"id"`
	RunID     string          `db:"run_id"`
	Code      string          `db:"code"`
	Symbol    string          `db:"symbol"`
	Profile   string          `db:"profile"`
	AsOf      time.Time       `db:"as_of"`
	Status    string          `db:"status"`
	Metrics   json.RawMessage `db:"metrics"`
	Synthesis json.RawMessage `db:"synthesis"`
	Markdown  *string         `db:"markdown"`
	Lint      json.RawMessage `db:"lint"`
	Gate      json.RawMessage `db:"gate"`
	Evidence  json.RawMessage `db:"evidence"`
	Error     *string         `db:"error"`
	Model     string          `db:"model"`
	Tokens    int64           `db:"tokens"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

// NormalizeCode research 的 full_code(sh600519/sz000560) → PIKS 的 6 位代码(600519)。
// 与 entities.detail->>'code' 对齐(设计 §4.6 归一规则);无前缀则原样返回。
func NormalizeCode(symbol string) string {
	s := strings.TrimSpace(strings.ToLower(symbol))
	for _, p := range []string{"sh", "sz", "bj"} {
		if strings.HasPrefix(s, p) && len(s) == len(p)+6 {
			return s[len(p):]
		}
	}
	return s
}

// CreateResearchRun 建行(status=pending);run_id 冲突时走幂等(同 run_id 不新增)。
// 返回是否新建(false = 已存在,调用方可跳过重跑)。
func (s *Store) CreateResearchRun(ctx context.Context, r *ResearchRun) (bool, error) {
	var created bool
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO research_runs(run_id, code, symbol, profile, as_of, status)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (run_id) DO NOTHING
		RETURNING true`,
		r.RunID, r.Code, r.Symbol, r.Profile, r.AsOf, defaultStr(r.Status, "pending")).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return created, err
}

// UpdateResearchRunAsOf 回写数据截止日(采集后以指标卡 meta.as_of 为准,防未来函数基准)。
func (s *Store) UpdateResearchRunAsOf(ctx context.Context, runID string, asOf time.Time) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE research_runs SET as_of=$2, updated_at=now() WHERE run_id=$1`, runID, asOf)
	return err
}

// UpdateResearchRunStatus 推进状态机;error 为空则清空错误列(重试成功不留旧错)。
func (s *Store) UpdateResearchRunStatus(ctx context.Context, runID, status, errMsg string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE research_runs SET status=$2, error=$3, updated_at=now() WHERE run_id=$1`,
		runID, status, nullIfEmpty(errMsg))
	return err
}

// SaveResearchArtifacts 落产物(各步逐步写,断点重跑可复用已产出的部分)。
// 传 nil 的字段保持原值不动(不覆盖已有成果)。
func (s *Store) SaveResearchArtifacts(ctx context.Context, runID string, r *ResearchRun) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE research_runs SET
		  metrics   = COALESCE($2, metrics),
		  synthesis = COALESCE($3, synthesis),
		  markdown  = COALESCE($4, markdown),
		  lint      = COALESCE($5, lint),
		  gate      = COALESCE($6, gate),
		  evidence  = COALESCE($7, evidence),
		  model     = COALESCE($8, model),
		  tokens    = COALESCE($9, tokens),
		  updated_at = now()
		WHERE run_id=$1`,
		runID, jsonOrNil(r.Metrics), jsonOrNil(r.Synthesis), r.Markdown,
		jsonOrNil(r.Lint), jsonOrNil(r.Gate), jsonOrNil(r.Evidence),
		nullIfEmpty(r.Model), nullIfZero(r.Tokens))
	return err
}

// FinishResearchRun 收尾:落状态 + 错误(失败如实留痕,不掩盖)。
func (s *Store) FinishResearchRun(ctx context.Context, runID, status, errMsg string, r *ResearchRun) error {
	if err := s.UpdateResearchRunStatus(ctx, runID, status, errMsg); err != nil {
		return err
	}
	if r != nil {
		return s.SaveResearchArtifacts(ctx, runID, r)
	}
	return nil
}

// GetResearchRun 按 run_id 取单份报告;无则 nil。
func (s *Store) GetResearchRun(ctx context.Context, runID string) (*ResearchRun, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+researchRunCols+` FROM research_runs WHERE run_id=$1`, runID)
	if err != nil {
		return nil, err
	}
	r, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ResearchRun])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

// ListResearchRuns 报告列表(code 非空则按股过滤),as_of DESC。
func (s *Store) ListResearchRuns(ctx context.Context, code string, limit int) ([]ResearchRun, error) {
	q := `SELECT ` + researchRunCols + ` FROM research_runs`
	args := []any{}
	if code != "" {
		args = append(args, NormalizeCode(code))
		q += ` WHERE code=$1`
	}
	q += ` ORDER BY as_of DESC, created_at DESC`
	if limit > 0 {
		q += ` LIMIT $` + strconv.Itoa(len(args)+1)
		args = append(args, limit)
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ResearchRun])
}

// ListResearchRunsByEntity 实体 → 报告列表(设计 §4.6 join:归一 code 对齐 entities.detail->>'code')。
func (s *Store) ListResearchRunsByEntity(ctx context.Context, entityID string) ([]ResearchRun, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+researchRunCols+` FROM research_runs r
		JOIN entities e ON e.type='company' AND e.detail->>'code' = r.code
		WHERE e.id=$1 ORDER BY r.as_of DESC, r.created_at DESC`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ResearchRun])
}

// jsonOrNil 空值转 NULL:配合 COALESCE 实现"只写本次产出的字段"(断点重跑不覆盖已有成果)。
func jsonOrNil(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

func nullIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
