package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// GNode 图谱节点。ID 带前缀区分事件/实体(e:/n:),Label 为真名/标题(非裸 UUID)。
type GNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // event | entity
	Type  string `json:"type"` // event_type 或实体 type
	URL   string `json:"url"`
}

// GEdge 图谱边。Source/Target 为节点 ID(带前缀)。
type GEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Rel    string `json:"rel"` // affects | belongs_to
}

// GraphData /api/graph 返回。
type GraphData struct {
	Nodes []GNode `json:"nodes"`
	Edges []GEdge `json:"edges"`
	Total struct {
		Events   int `json:"events"`
		Entities int `json:"entities"`
	} `json:"total"`
	Focus string `json:"focus,omitempty"`
}

func (s *Server) handleGraphAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	focus := q.Get("focus")
	scope := q.Get("scope")
	limit := 40
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	data, err := s.graphData(r.Context(), focus, scope, limit)
	if err != nil {
		s.writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, data)
}

// graphData 组装图谱:局部优先(默认最近 limit 事件邻域;focus 时取该节点邻域)。
func (s *Server) graphData(ctx context.Context, focus, scope string, limit int) (GraphData, error) {
	evs, err := s.store.ListEventsForPublishWithSource(ctx)
	if err != nil {
		return GraphData{}, err
	}
	ents, err := s.store.ListAllEntities(ctx)
	if err != nil {
		return GraphData{}, err
	}
	affects, err := s.store.ListGraphEdges(ctx)
	if err != nil {
		return GraphData{}, err
	}
	belongs, err := s.store.ListRelationshipsFromTo(ctx, "entity", "entity", "belongs_to")
	if err != nil {
		return GraphData{}, err
	}

	eventByID := map[string]*store.EventForPublish{}
	for i := range evs {
		eventByID[evs[i].ID] = &evs[i]
	}
	entityByID := map[string]*model.Entity{}
	for i := range ents {
		entityByID[ents[i].ID] = &ents[i]
	}
	affectsOfEvent := map[string][]string{}
	eventsAffecting := map[string][]string{}
	for _, a := range affects {
		affectsOfEvent[a.EventID] = append(affectsOfEvent[a.EventID], a.EntityID)
		eventsAffecting[a.EntityID] = append(eventsAffecting[a.EntityID], a.EventID)
	}
	belongsOf := map[string][]string{}
	for _, b := range belongs {
		belongsOf[b.FromID] = append(belongsOf[b.FromID], b.ToID)
		belongsOf[b.ToID] = append(belongsOf[b.ToID], b.FromID)
	}

	eventNode := func(e *store.EventForPublish) GNode {
		return GNode{ID: "e:" + e.ID, Label: e.Title, Kind: "event", Type: e.EventType, URL: "/events/" + e.ID}
	}
	entityNode := func(e *model.Entity) GNode {
		return GNode{ID: "n:" + e.ID, Label: e.Name, Kind: "entity", Type: e.Type, URL: "/entities/" + e.ID}
	}

	g := GraphData{}
	g.Total.Events = len(evs)
	g.Total.Entities = len(ents)
	nodeSeen := map[string]bool{}
	edgeSeen := map[string]bool{}
	addNode := func(n GNode) {
		if !nodeSeen[n.ID] {
			nodeSeen[n.ID] = true
			g.Nodes = append(g.Nodes, n)
		}
	}
	addEdge := func(src, dst, rel string) {
		key := src + "|" + dst + "|" + rel
		if edgeSeen[key] {
			return
		}
		edgeSeen[key] = true
		g.Edges = append(g.Edges, GEdge{Source: src, Target: dst, Rel: rel})
	}
	// addEventNeighborhood 事件 + 其受影响实体 + 实体所属行业(belongs),边含 affects/belongs。
	addEventNeighborhood := func(e *store.EventForPublish, maxBelongs int) {
		addNode(eventNode(e))
		for _, entID := range affectsOfEvent[e.ID] {
			ent, ok := entityByID[entID]
			if !ok {
				continue
			}
			addNode(entityNode(ent))
			addEdge(eventNode(e).ID, entityNode(ent).ID, "affects")
			// 实体的 belongs 邻居(行业→公司等)
			for i, partner := range belongsOf[ent.ID] {
				if i >= maxBelongs {
					break
				}
				p, ok := entityByID[partner]
				if !ok {
					continue
				}
				addNode(entityNode(p))
				addEdge(entityNode(ent).ID, entityNode(p).ID, "belongs_to")
			}
		}
	}

	if focus != "" {
		kind, id, _ := strings.Cut(focus, ":")
		switch kind {
		case "event":
			if e, ok := eventByID[id]; ok {
				addEventNeighborhood(e, 8)
				g.Focus = focus
			}
		case "entity":
			if ent, ok := entityByID[id]; ok {
				addNode(entityNode(ent))
				// 影响它的事件
				for _, evID := range eventsAffecting[ent.ID] {
					if e, ok := eventByID[evID]; ok {
						addNode(eventNode(e))
						addEdge(eventNode(e).ID, entityNode(ent).ID, "affects")
					}
				}
				// belongs 邻居
				for i, partner := range belongsOf[ent.ID] {
					if i >= 8 {
						break
					}
					if p, ok := entityByID[partner]; ok {
						addNode(entityNode(p))
						addEdge(entityNode(ent).ID, entityNode(p).ID, "belongs_to")
					}
				}
				g.Focus = focus
			}
		}
		if len(g.Nodes) == 0 {
			return g, nil
		}
		return g, nil
	}

	if scope == "global" {
		for i := range evs {
			addNode(eventNode(&evs[i]))
		}
		for i := range ents {
			addNode(entityNode(&ents[i]))
		}
		for _, a := range affects {
			e := eventByID[a.EventID]
			n := entityByID[a.EntityID]
			if e == nil || n == nil {
				continue // 悬空边(事件/实体已删),诚实跳过
			}
			addEdge("e:"+e.ID, "n:"+n.ID, "affects")
		}
		for _, b := range belongs {
			f := entityByID[b.FromID]
			t := entityByID[b.ToID]
			if f == nil || t == nil {
				continue
			}
			addEdge("n:"+f.ID, "n:"+t.ID, "belongs_to")
		}
		return g, nil
	}

	// 默认:最近 limit 个事件邻域
	recent := recentEvents(evs, limit)
	for _, e := range recent {
		addEventNeighborhood(e, 6)
	}
	return g, nil
}

// recentEvents 按日期(occurred_at ?? created_at)倒序取前 n。
func recentEvents(evs []store.EventForPublish, n int) []*store.EventForPublish {
	out := make([]*store.EventForPublish, 0, len(evs))
	for i := range evs {
		out = append(out, &evs[i])
	}
	sort.Slice(out, func(i, j int) bool {
		return eventKey(out[i]) > eventKey(out[j])
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func eventKey(e *store.EventForPublish) string {
	if e.OccurredAt != nil {
		return e.OccurredAt.Format(time.RFC3339)
	}
	return e.CreatedAt.Format(time.RFC3339)
}
