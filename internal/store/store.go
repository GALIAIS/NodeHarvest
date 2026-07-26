package store

import (
	"crypto/sha1" // #nosec G505 -- retained only for backward-compatible, non-cryptographic node IDs.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// Store 内存 + 磁盘快照
type Store struct {
	mu         sync.RWMutex
	nodes      map[string]*model.Node
	jobs       map[string]*model.Job
	jobOrder   []string
	dataDir    string
	lastFetch  *time.Time
	lastQual   *time.Time
	hostAI     map[string]*model.AIProbeResult
	persistent bool
}

func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	// #nosec G302 -- dataDir is a directory and 0700 is the restrictive directory mode.
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		nodes:      make(map[string]*model.Node),
		jobs:       make(map[string]*model.Job),
		dataDir:    dataDir,
		hostAI:     make(map[string]*model.AIProbeResult),
		persistent: true,
	}
	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	return s, nil
}

func NewMemory() *Store {
	return &Store{
		nodes:  make(map[string]*model.Node),
		jobs:   make(map[string]*model.Job),
		hostAI: make(map[string]*model.AIProbeResult),
	}
}

func nodeID(n *model.Node) string {
	// #nosec G401 -- changing this legacy non-security digest would orphan persisted node history.
	h := sha1.Sum([]byte(n.Key()))
	return hex.EncodeToString(h[:8])
}

func cloneNode(n *model.Node) *model.Node {
	if n == nil {
		return nil
	}
	cp := *n
	cp.Sources = append([]string(nil), n.Sources...)
	cp.Tags = append([]string(nil), n.Tags...)
	if n.Extra != nil {
		cp.Extra = make(map[string]string, len(n.Extra))
		for key, value := range n.Extra {
			cp.Extra[key] = value
		}
	}
	if n.Quality != nil {
		q := *n.Quality
		q.Notes = append([]string(nil), n.Quality.Notes...)
		if n.Quality.Breakdown != nil {
			q.Breakdown = make(map[string]float64, len(n.Quality.Breakdown))
			for key, value := range n.Quality.Breakdown {
				q.Breakdown[key] = value
			}
		}
		cp.Quality = &q
	}
	if n.AIAccess != nil {
		cp.AIAccess = make(map[string]*model.AIProbeResult, len(n.AIAccess))
		for key, value := range n.AIAccess {
			if value != nil {
				result := *value
				cp.AIAccess[key] = &result
			}
		}
	}
	if n.Dial != nil {
		result := *n.Dial
		cp.Dial = &result
	}
	if n.Purity != nil {
		result := *n.Purity
		result.Notes = append([]string(nil), n.Purity.Notes...)
		cp.Purity = &result
	}
	return &cp
}

func cloneJob(j *model.Job) *model.Job {
	if j == nil {
		return nil
	}
	cp := *j
	if j.Stats != nil {
		cp.Stats = make(map[string]any, len(j.Stats))
		for key, value := range j.Stats {
			cp.Stats[key] = value
		}
	}
	if j.Options != nil {
		cp.Options = make(map[string]any, len(j.Options))
		for key, value := range j.Options {
			cp.Options[key] = value
		}
	}
	return &cp
}

func (s *Store) ReplaceNodes(nodes []*model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldByKey := make(map[string]*model.Node, len(s.nodes))
	for _, n := range s.nodes {
		oldByKey[n.Key()] = n
	}
	s.nodes = make(map[string]*model.Node, len(nodes))
	now := time.Now()
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if old := oldByKey[n.Key()]; old != nil {
			mergeObservations(n, old)
		}
		if n.FirstSeenAt.IsZero() {
			n.FirstSeenAt = now
		}
		if n.LastSeenAt.IsZero() {
			n.LastSeenAt = now
		}
		id := nodeID(n)
		n.ID = id
		n.Fingerprint = n.Key()
		s.nodes[id] = cloneNode(n)
	}
	s.lastFetch = &now
	return s.persistLocked()
}

func mergeObservations(dst, old *model.Node) {
	if dst.FirstSeenAt.IsZero() {
		dst.FirstSeenAt = old.FirstSeenAt
	}
	if dst.LastSeenAt.IsZero() {
		dst.LastSeenAt = old.LastSeenAt
	}
	if dst.TestedAt.IsZero() {
		dst.Alive = old.Alive
		dst.Latency = old.Latency
		dst.Error = old.Error
		dst.TestedAt = old.TestedAt
		dst.Quality = old.Quality
		dst.Score = old.Score
		dst.Grade = old.Grade
		dst.Tags = old.Tags
		dst.QualityFailures = old.QualityFailures
		dst.SuccessStreak = old.SuccessStreak
		dst.NextTestAt = old.NextTestAt
	}
	if dst.Country == "" {
		dst.Country = old.Country
		dst.City = old.City
		dst.ISP = old.ISP
		dst.ASN = old.ASN
		dst.EntryType = old.EntryType
	}
	if dst.AIAccess == nil {
		dst.AIAccess = old.AIAccess
	}
	if dst.Dial == nil {
		dst.Dial = old.Dial
		dst.Verified = old.Verified
	}
	if dst.Purity == nil {
		dst.Purity = old.Purity
	}
}

func (s *Store) UpsertNodes(nodes []*model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range nodes {
		id := nodeID(n)
		n.ID = id
		n = cloneNode(n)
		if old, ok := s.nodes[id]; ok {
			old.Alive = n.Alive
			old.Latency = n.Latency
			old.Error = n.Error
			old.TestedAt = n.TestedAt
			old.Quality = n.Quality
			old.AIAccess = n.AIAccess
			old.Score = n.Score
			old.Grade = n.Grade
			old.QualityFailures = n.QualityFailures
			old.SuccessStreak = n.SuccessStreak
			old.NextTestAt = n.NextTestAt
			if !n.FirstSeenAt.IsZero() {
				old.FirstSeenAt = n.FirstSeenAt
			}
			if !n.LastSeenAt.IsZero() {
				old.LastSeenAt = n.LastSeenAt
			}
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
			if n.ASN != "" {
				old.ASN = n.ASN
			}
			if n.EntryType != "" {
				old.EntryType = n.EntryType
			}
			if n.Dial != nil {
				old.Dial = n.Dial
				old.Verified = n.Verified
			}
			if n.Purity != nil {
				old.Purity = n.Purity
			}
			// 允许拨测结果把 Verified 写回（即使无新 Dial 指针变化）
			if n.Verified {
				old.Verified = true
			}
			if n.RawURI != "" {
				old.RawURI = n.RawURI
			}
		} else {
			s.nodes[id] = n
		}
	}
	now := time.Now()
	s.lastQual = &now
	return s.persistLocked()
}

func (s *Store) ListNodes(filter NodeFilter) []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if !filter.Match(n) {
			continue
		}
		out = append(out, cloneNode(n))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Latency != out[j].Latency {
			return out[i].Latency < out[j].Latency
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
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
	return cloneNode(n), true
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked()
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

func (s *Store) SetHostAI(m map[string]*model.AIProbeResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostAI = m
	return s.persistLocked()
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
	s.jobs[j.ID] = cloneJob(j)
}

func (s *Store) GetJob(id string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(j), true
}

func (s *Store) ListJobs(limit int) []*model.Job {
	return s.ListJobsPage(limit, "")
}

func (s *Store) ListJobsPage(limit int, cursor string) []*model.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]*model.Job, 0, limit)
	started := cursor == ""
	for i := len(s.jobOrder) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.jobOrder[i]
		if !started {
			if id == cursor {
				started = true
			}
			continue
		}
		if j, ok := s.jobs[id]; ok {
			out = append(out, cloneJob(j))
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
	SeenAfter    time.Time
}

func (f NodeFilter) Match(n *model.Node) bool {
	if f.Protocol != "" && string(n.Protocol) != f.Protocol {
		return false
	}
	if f.Source != "" && n.Source != f.Source && !slices.Contains(n.Sources, f.Source) {
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
	if !f.SeenAfter.IsZero() && !n.LastSeenAt.IsZero() && n.LastSeenAt.Before(f.SeenAfter) {
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
		blob := strings.ToLower(n.Name + " " + n.Server + " " + n.Source + " " +
			strings.Join(n.Sources, " ") + " " + string(n.Protocol) + " " + n.Country + " " + n.City)
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
	if !s.persistent {
		return nil
	}
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
	path := filepath.Join(s.dataDir, "snapshot.json")
	tmp, err := os.CreateTemp(s.dataDir, "snapshot-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	// Windows 不能原子覆盖已有文件；生产 Linux 走上面的原子 rename。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *Store) Load() error {
	path := filepath.Join(s.dataDir, "snapshot.json")
	// #nosec G304 -- path is fixed to snapshot.json under the private configured data directory.
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
		n.ID = nodeID(n)
		n.Fingerprint = n.Key()
		if n.FirstSeenAt.IsZero() {
			n.FirstSeenAt = time.Now()
		}
		if n.LastSeenAt.IsZero() {
			n.LastSeenAt = n.FirstSeenAt
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
func (s *Store) Prune(opt PruneOptions) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := len(s.nodes)
	kept := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if n == nil {
			continue
		}
		tested := !n.TestedAt.IsZero()
		if opt.DropDead && tested && !n.Alive && n.QualityFailures >= 3 {
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
	err := s.persistLocked()
	return before - len(s.nodes), err
}
