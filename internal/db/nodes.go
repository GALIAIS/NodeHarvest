package db

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

type NodeMetric struct {
	NodeID        string  `json:"node_id,omitempty"`
	MeasuredAt    string  `json:"measured_at"`
	Success       bool    `json:"success"`
	LatencyMS     int64   `json:"latency_ms"`
	JitterMS      int64   `json:"jitter_ms"`
	TLSMS         int64   `json:"tls_ms"`
	HTTPMS        int64   `json:"http_ms"`
	ThroughputBPS int64   `json:"throughput_bps"`
	Score         float64 `json:"score"`
}

type DailyMetric struct {
	Day           string  `json:"day"`
	Samples       int     `json:"samples"`
	SuccessRate   float64 `json:"success_rate"`
	P50LatencyMS  int64   `json:"p50_latency_ms"`
	P95LatencyMS  int64   `json:"p95_latency_ms"`
	AvgScore      float64 `json:"avg_score"`
	AvgThroughput int64   `json:"avg_throughput_bps"`
}

func (s *Store) SaveNodes(nodes []*model.Node) error {
	if s == nil || len(nodes) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	query := s.rebind(`
INSERT INTO nodes(id,fingerprint,protocol,server,port,country,asn,entry_type,score,alive,first_seen_at,last_seen_at,payload_json,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 fingerprint=excluded.fingerprint, protocol=excluded.protocol, server=excluded.server, port=excluded.port,
 country=excluded.country, asn=excluded.asn, entry_type=excluded.entry_type, score=excluded.score,
 alive=excluded.alive, first_seen_at=excluded.first_seen_at, last_seen_at=excluded.last_seen_at,
 payload_json=excluded.payload_json, updated_at=excluded.updated_at`)
	now := fmtTime(time.Now())
	for _, node := range nodes {
		if node == nil || node.ID == "" {
			continue
		}
		payload, marshalErr := json.Marshal(node)
		if marshalErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal node %s: %w", node.ID, marshalErr)
		}
		alive := 0
		if node.Alive {
			alive = 1
		}
		if _, execErr := tx.Exec(query, node.ID, node.Fingerprint, string(node.Protocol), node.Server, node.Port,
			node.Country, node.ASN, node.EntryType, node.Score, alive, fmtTime(node.FirstSeenAt),
			fmtTime(node.LastSeenAt), string(payload), now); execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
	}
	return tx.Commit()
}

// ReplaceNodes makes the durable node set match the current hot snapshot.
func (s *Store) ReplaceNodes(nodes []*model.Node) error {
	if err := s.SaveNodes(nodes); err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if node != nil && node.ID != "" {
			keep[node.ID] = struct{}{}
		}
	}
	rows, err := s.query(`SELECT id FROM nodes`)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		if _, ok := keep[id]; !ok {
			stale = append(stale, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := tx.Exec(s.rebind(`DELETE FROM nodes WHERE id=?`), id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadNodes(limit int) ([]*model.Node, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.query(`SELECT payload_json FROM nodes ORDER BY last_seen_at DESC,id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.Node, 0, limit)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var node model.Node
		if err := json.Unmarshal([]byte(payload), &node); err != nil {
			return nil, fmt.Errorf("decode persisted node: %w", err)
		}
		out = append(out, &node)
	}
	return out, rows.Err()
}

func (s *Store) RecordNodeMetrics(nodes []*model.Node) error {
	if s == nil || len(nodes) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	query := s.rebind(`INSERT INTO node_metrics(node_id,measured_at,success,latency_ms,jitter_ms,tls_ms,http_latency_ms,throughput_bps,score)
VALUES(?,?,?,?,?,?,?,?,?)`)
	for _, node := range nodes {
		if node == nil || node.ID == "" || (node.TestedAt.IsZero() && node.Dial == nil) {
			continue
		}
		metric := metricFromNode(node)
		success := 0
		if metric.Success {
			success = 1
		}
		if _, execErr := tx.Exec(query, node.ID, metric.MeasuredAt, success, metric.LatencyMS, metric.JitterMS,
			metric.TLSMS, metric.HTTPMS, metric.ThroughputBPS, metric.Score); execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
	}
	return tx.Commit()
}

func metricFromNode(node *model.Node) NodeMetric {
	at := node.TestedAt
	if node.Dial != nil && node.Dial.TestedAt.After(at) {
		at = node.Dial.TestedAt
	}
	if at.IsZero() {
		at = time.Now()
	}
	metric := NodeMetric{
		NodeID:     node.ID,
		MeasuredAt: fmtTime(at),
		Success:    node.Alive,
		LatencyMS:  node.LatencyMS(),
		Score:      node.Score,
	}
	if node.Quality != nil {
		metric.JitterMS = node.Quality.JitterMS
		metric.TLSMS = node.Quality.TLSMS
		metric.HTTPMS = node.Quality.HTTPMS
		metric.ThroughputBPS = node.Quality.ThroughputBPS
	}
	if node.Dial != nil {
		metric.Success = node.Dial.OK
		metric.HTTPMS = node.Dial.HTTPMS
		metric.ThroughputBPS = node.Dial.ThroughputBPS
		if metric.LatencyMS == 0 {
			metric.LatencyMS = node.Dial.LatencyMS
		}
	}
	return metric
}

func (s *Store) DailyNodeMetrics(nodeID string, days int) ([]DailyMetric, error) {
	if s == nil {
		return nil, nil
	}
	if days <= 0 {
		days = 30
	}
	cutoff := fmtTime(time.Now().AddDate(0, 0, -days))
	query := `SELECT measured_at,success,latency_ms,score,throughput_bps FROM node_metrics WHERE measured_at>=?`
	args := []any{cutoff}
	if nodeID != "" {
		query += ` AND node_id=?`
		args = append(args, nodeID)
	}
	query += ` ORDER BY measured_at`
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type bucket struct {
		latencies  []int64
		samples    int
		successes  int
		score      float64
		throughput int64
	}
	buckets := make(map[string]*bucket)
	for rows.Next() {
		var at string
		var success int
		var latency, throughput int64
		var score float64
		if err := rows.Scan(&at, &success, &latency, &score, &throughput); err != nil {
			return nil, err
		}
		day := at
		if len(day) > 10 {
			day = day[:10]
		}
		b := buckets[day]
		if b == nil {
			b = &bucket{}
			buckets[day] = b
		}
		b.samples++
		if success == 1 {
			b.successes++
		}
		if latency > 0 {
			b.latencies = append(b.latencies, latency)
		}
		b.score += score
		b.throughput += throughput
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	daysList := make([]string, 0, len(buckets))
	for day := range buckets {
		daysList = append(daysList, day)
	}
	sort.Strings(daysList)
	out := make([]DailyMetric, 0, len(daysList))
	for _, day := range daysList {
		b := buckets[day]
		sort.Slice(b.latencies, func(i, j int) bool { return b.latencies[i] < b.latencies[j] })
		out = append(out, DailyMetric{
			Day:           day,
			Samples:       b.samples,
			SuccessRate:   float64(b.successes) / float64(b.samples),
			P50LatencyMS:  percentile(b.latencies, 0.50),
			P95LatencyMS:  percentile(b.latencies, 0.95),
			AvgScore:      b.score / float64(b.samples),
			AvgThroughput: b.throughput / int64(b.samples),
		})
	}
	return out, nil
}

// NodeStabilities returns per-node success ratios for the requested history window.
func (s *Store) NodeStabilities(days int) (map[string]float64, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := s.query(`SELECT node_id,AVG(success) FROM node_metrics
 WHERE measured_at>=? GROUP BY node_id`, fmtTime(time.Now().AddDate(0, 0, -days)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var nodeID string
		var stability float64
		if err := rows.Scan(&nodeID, &stability); err != nil {
			return nil, err
		}
		out[nodeID] = stability
	}
	return out, rows.Err()
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func (s *Store) DeleteNodesMissingSince(cutoff time.Time) (int64, error) {
	if s == nil {
		return 0, nil
	}
	result, err := s.exec(`DELETE FROM nodes WHERE last_seen_at < ?`, fmtTime(cutoff))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
