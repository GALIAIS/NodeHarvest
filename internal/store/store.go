package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/local/node-hunter/internal/model"
)

// Store 内存 + 磁盘快照
type Store struct {
	mu        sync.RWMutex
	nodes     map[string]*model.Node
	jobs      map[string]*model.Job
	jobOrder  []string
	dataDir   string
	lastFetch *time.Time
	lastQual  *time.Time
	hostAI    map[string]*model.AIProbeResult
}

func New(dataDir string) *Store {
	if dataDir == "" {
		dataDir = "data"
	}
	_ = os.MkdirAll(dataDir, 0o755)
	s := &Store{
		nodes:   make(map[string]*model.Node),
		jobs:    make(map[string]*model.Job),
		dataDir: dataDir,
		hostAI:  make(map[string]*model.AIProbeResult),
	}
	_ = s.Load()
	return s
}

func nodeID(n *model.Node) string {
	if n.ID != "" {
		return n.ID
	}
	h := sha1.Sum([]byte(n.Key() + "|" + n.RawURI))
	return hex.EncodeToString(h[:8])
}

func (s *Store) ReplaceNodes(nodes []*model.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = make(map[string]*model.Node, len(nodes))
	for _, n := range nodes {
		id := nodeID(n)
		n.ID = id
		n.Fingerprint = n.Key()
		cp := *n
		s.nodes[id] = &cp
	}
	now := time.Now()
	s.lastFetch = &now
	_ = s.persistLocked()
}

func (s *Store) UpsertNodes(nodes []*model.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range nodes {
		id := nodeID(n)
		n.ID = id
		if old, ok := s.nodes[id]; ok {
			old.Alive = n.Alive
			old.Latency = n.Latency
			old.Error = n.Error
			old.TestedAt = n.TestedAt
			old.Quality = n.Quality
			old.AIAccess = n.AIAccess
			old.Score = n.Score
			old.Grade = n.Grade
			if len(n.Tags) > 0 {
				old.Tags = n.Tags
			}
			// 地理信息：有新值才覆盖，避免空写抹掉
			if n.Country != "" {
				old.Country = n.Country
			}
			if n.City != "" {
				old.City = n.City
			}
			if n.ISP != "" {
				old.ISP = n.ISP
			}
			if n.Dial != nil {
				old.Dial = n.Dial
				old.Verified = n.Verified
			}
			if n.RawURI != "" {
				old.RawURI = n.RawURI
			}
		} else {
			cp := *n
			s.nodes[id] = &cp
		}
	}
	now := time.Now()
	s.lastQual = &now
	_ = s.persistLocked()
}

func (s *Store) ListNodes(filter NodeFilter) []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if !filter.Match(n) {
			continue
		}
		cp := *n
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Latency != out[j].Latency {
			return out[i].Latency < out[j].Latency
		}
		return out[i].Name < out[j].Name
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out
}

func (s *Store) GetNode(id string) (*model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil, false
	}
	cp := *n
	return &cp, true
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

func (s *Store) AllNodes() []*model.Node {
	return s.ListNodes(NodeFilter{})
}

func (s *Store) Stats(sourcesEnabled int) model.DashboardStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := model.DashboardStats{
		ByProtocol:     map[string]int{},
		ByGrade:        map[string]int{},
		BySource:       map[string]int{},
		ByCountry:      map[string]int{},
		ByCountryHQ:    map[string]int{},
		AIPassRate:     map[string]float64{},
		SourcesEnabled: sourcesEnabled,
		LastFetchAt:    s.lastFetch,
		LastQualityAt:  s.lastQual,
	}
	var latSum int64
	latN := 0
	aiOK := map[string]int{}
	aiTotal := map[string]int{}

	for _, n := range s.nodes {
		st.TotalNodes++
		if n.Alive {
			st.AliveNodes++
		}
		if n.Score >= 70 {
			st.HighQuality++
		}
		st.ByProtocol[string(n.Protocol)]++
		if n.Grade != "" {
			st.ByGrade[n.Grade]++
		}
		if n.Source != "" {
			st.BySource[n.Source]++
		}
		cc := n.Country
		if cc == "" {
			cc = "XX"
		}
		st.ByCountry[cc]++
		if n.Alive && n.Score >= 70 {
			st.ByCountryHQ[cc]++
		}
		if n.Alive && n.LatencyMS() > 0 {
			latSum += n.LatencyMS()
			latN++
		}
		for k, r := range n.AIAccess {
			aiTotal[k]++
			if r != nil && r.OK {
				aiOK[k]++
			}
		}
	}
	if latN > 0 {
		st.AvgLatencyMS = latSum / int64(latN)
	}
	for k, total := range aiTotal {
		if total > 0 {
			st.AIPassRate[k] = float64(aiOK[k]) / float64(total)
		}
	}
	return st
}

func (s *Store) SetHostAI(m map[string]*model.AIProbeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostAI = m
	_ = s.persistLocked()
}

func (s *Store) HostAI() map[string]*model.AIProbeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*model.AIProbeResult, len(s.hostAI))
	for k, v := range s.hostAI {
		cp := *v
		out[k] = &cp
	}
	return out
}

func (s *Store) SaveJob(j *model.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[j.ID]; !ok {
		s.jobOrder = append(s.jobOrder, j.ID)
	}
	cp := *j
	s.jobs[j.ID] = &cp
}

func (s *Store) GetJob(id string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	return &cp, true
}

func (s *Store) ListJobs(limit int) []*model.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]*model.Job, 0, limit)
	for i := len(s.jobOrder) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.jobOrder[i]
		if j, ok := s.jobs[id]; ok {
			cp := *j
			out = append(out, &cp)
		}
	}
	return out
}

// NodeFilter 列表过滤
type NodeFilter struct {
	Protocol     string
	Source       string
	Grade        string
	Country      string // ISO 国家码，如 US / JP；XX=未知
	AliveOnly    bool
	MinScore     float64
	AIKey        string
	Search       string
	Limit        int
	HighQuality  bool
	VerifiedOnly bool // 仅真实拨测通过
}

func (f NodeFilter) Match(n *model.Node) bool {
	if f.Protocol != "" && string(n.Protocol) != f.Protocol {
		return false
	}
	if f.Source != "" && n.Source != f.Source {
		return false
	}
	if f.Grade != "" && n.Grade != f.Grade {
		return false
	}
	if f.Country != "" {
		want := strings.ToUpper(f.Country)
		got := strings.ToUpper(n.Country)
		if got == "" {
			got = "XX"
		}
		if want == "UK" {
			want = "GB"
		}
		if got == "UK" {
			got = "GB"
		}
		if got != want {
			return false
		}
	}
	if f.AliveOnly && !n.Alive {
		return false
	}
	if f.MinScore > 0 && n.Score < f.MinScore {
		return false
	}
	if f.HighQuality && n.Score < 70 {
		return false
	}
	if f.VerifiedOnly && !n.Verified {
		return false
	}
	if f.AIKey != "" {
		r, ok := n.AIAccess[f.AIKey]
		if !ok || r == nil || !r.OK {
			return false
		}
	}
	if f.Search != "" {
		q := strings.ToLower(f.Search)
		blob := strings.ToLower(n.Name + " " + n.Server + " " + n.Source + " " + string(n.Protocol) + " " + n.Country + " " + n.City)
		if !strings.Contains(blob, q) {
			return false
		}
	}
	return true
}

type snapshot struct {
	Nodes     []*model.Node                   `json:"nodes"`
	HostAI    map[string]*model.AIProbeResult `json:"host_ai"`
	LastFetch *time.Time                      `json:"last_fetch"`
	LastQual  *time.Time                      `json:"last_quality"`
}

func (s *Store) persistLocked() error {
	nodes := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Score != nodes[j].Score {
			return nodes[i].Score > nodes[j].Score
		}
		return nodes[i].Latency < nodes[j].Latency
	})
	// 默认上限；Service 侧 prune 会再约束
	if len(nodes) > 12000 {
		nodes = nodes[:12000]
	}
	snap := snapshot{
		Nodes:     nodes,
		HostAI:    s.hostAI,
		LastFetch: s.lastFetch,
		LastQual:  s.lastQual,
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "snapshot.json"), b, 0o644)
}

func (s *Store) Load() error {
	path := filepath.Join(s.dataDir, "snapshot.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = make(map[string]*model.Node, len(snap.Nodes))
	for _, n := range snap.Nodes {
		if n.ID == "" {
			n.ID = nodeID(n)
		}
		s.nodes[n.ID] = n
	}
	if snap.HostAI != nil {
		s.hostAI = snap.HostAI
	}
	s.lastFetch = snap.LastFetch
	s.lastQual = snap.LastQual
	return nil
}

func (s *Store) ExportURIs(filter NodeFilter) []string {
	nodes := s.ListNodes(filter)
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.RawURI != "" {
			out = append(out, n.RawURI)
		}
	}
	return out
}

// PruneOptions 清理策略
type PruneOptions struct {
	DropDead     bool
	MinScoreKeep float64 // >0 时丢弃低于该分且已测过的节点
	MaxNodes     int
	KeepUntested bool
}

// Prune 清理低质量/死亡节点，返回删除数量
func (s *Store) Prune(opt PruneOptions) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := len(s.nodes)
	kept := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if n == nil {
			continue
		}
		tested := !n.TestedAt.IsZero()
		if opt.DropDead && tested && !n.Alive {
			continue
		}
		if opt.MinScoreKeep > 0 && tested && n.Score < opt.MinScoreKeep {
			// 未测保留可选
			if !(opt.KeepUntested && !tested) {
				continue
			}
		}
		if !tested && !opt.KeepUntested && opt.DropDead {
			// 仅在明确不要未测时丢
		}
		kept = append(kept, n)
	}

	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Alive != kept[j].Alive {
			return kept[i].Alive
		}
		if kept[i].Score != kept[j].Score {
			return kept[i].Score > kept[j].Score
		}
		return kept[i].Latency < kept[j].Latency
	})
	if opt.MaxNodes > 0 && len(kept) > opt.MaxNodes {
		kept = kept[:opt.MaxNodes]
	}

	s.nodes = make(map[string]*model.Node, len(kept))
	for _, n := range kept {
		s.nodes[n.ID] = n
	}
	_ = s.persistLocked()
	return before - len(s.nodes)
}
