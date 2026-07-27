package db

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/timex"
)

type Alert struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	Message        string         `json:"message"`
	Details        map[string]any `json:"details,omitempty"`
	Active         bool           `json:"active"`
	CreatedAt      string         `json:"created_at"`
	ResolvedAt     string         `json:"resolved_at,omitempty"`
	AcknowledgedAt string         `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string         `json:"acknowledged_by,omitempty"`
}

func (s *Store) RaiseAlert(kind, severity, message string, details map[string]any) (*Alert, error) {
	payload, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	at := timex.NowRFC3339()
	_, err = s.exec(`INSERT INTO alerts(
 id,kind,severity,message,details_json,active,created_at,resolved_at,acknowledged_at,acknowledged_by
) VALUES(?,?,?,?,?,1,?,NULL,NULL,NULL)
ON CONFLICT(id) DO UPDATE SET severity=excluded.severity,message=excluded.message,
 details_json=excluded.details_json,active=1,created_at=excluded.created_at,
 resolved_at=NULL,acknowledged_at=NULL,acknowledged_by=NULL`,
		kind, kind, severity, message, string(payload), at)
	if err != nil {
		return nil, err
	}
	return &Alert{
		ID: kind, Kind: kind, Severity: severity, Message: message, Details: details, Active: true, CreatedAt: at,
	}, nil
}

func (s *Store) ListAlerts(activeOnly bool, limit int) ([]Alert, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id,kind,severity,message,details_json,active,created_at,resolved_at,acknowledged_at,acknowledged_by
 FROM alerts`
	if activeOnly {
		query += ` WHERE active=1`
	}
	query += ` ORDER BY active DESC,created_at DESC,id DESC LIMIT ?`
	rows, err := s.query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlerts(rows)
}

func (s *Store) ListAlertsPage(activeOnly bool, limit int, cursor string) ([]Alert, error) {
	if limit <= 0 || limit > 101 {
		limit = 31
	}
	query := `SELECT id,kind,severity,message,details_json,active,created_at,resolved_at,acknowledged_at,acknowledged_by
 FROM alerts`
	conditions := []string{}
	args := []any{}
	if activeOnly {
		conditions = append(conditions, `active=1`)
	}
	if cursor != "" {
		var active int
		var created string
		cursorQuery := `SELECT active,created_at FROM alerts WHERE id=?`
		cursorArgs := []any{cursor}
		if activeOnly {
			cursorQuery += ` AND active=1`
		}
		if err := s.queryRow(cursorQuery, cursorArgs...).Scan(&active, &created); err != nil {
			return nil, err
		}
		conditions = append(conditions, `(active<? OR (active=? AND (created_at<? OR (created_at=? AND id<?))))`)
		args = append(args, active, active, created, created, cursor)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY active DESC,created_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlerts(rows)
}

func (s *Store) CountAlerts(activeOnly bool) (int, error) {
	query := `SELECT COUNT(*) FROM alerts`
	if activeOnly {
		query += ` WHERE active=1`
	}
	var count int
	if err := s.queryRow(query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanAlerts(rows *sql.Rows) ([]Alert, error) {
	var out []Alert
	for rows.Next() {
		var alert Alert
		var payload string
		var active int
		var resolved, acknowledgedAt, acknowledgedBy sql.NullString
		if err := rows.Scan(&alert.ID, &alert.Kind, &alert.Severity, &alert.Message, &payload, &active,
			&alert.CreatedAt, &resolved, &acknowledgedAt, &acknowledgedBy); err != nil {
			return nil, err
		}
		alert.Active = active == 1
		alert.ResolvedAt = resolved.String
		alert.AcknowledgedAt = acknowledgedAt.String
		alert.AcknowledgedBy = acknowledgedBy.String
		_ = json.Unmarshal([]byte(payload), &alert.Details)
		out = append(out, alert)
	}
	return out, rows.Err()
}

func (s *Store) AcknowledgeAlert(id, actor string) error {
	result, err := s.exec(`UPDATE alerts SET acknowledged_at=?,acknowledged_by=? WHERE id=? AND active=1`,
		timex.NowRFC3339(), actor, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ResolveAlert(id string) error {
	result, err := s.exec(`UPDATE alerts SET active=0,resolved_at=? WHERE id=? AND active=1`,
		timex.NowRFC3339(), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ResolveAlertIfActive(id string) {
	_, _ = s.exec(`UPDATE alerts SET active=0,resolved_at=? WHERE id=? AND active=1`, timex.NowRFC3339(), id)
}

func (s *Store) PreviousCompletedQualityStats(excludeID string) (map[string]any, error) {
	row := s.queryRow(`SELECT stats_json FROM jobs
 WHERE id<>? AND status='completed' AND (type='quality' OR type='full')
 ORDER BY ended_at DESC,id DESC LIMIT 1`, excludeID)
	var payload string
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	var stats map[string]any
	if err := json.Unmarshal([]byte(payload), &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *Store) RecentConsecutiveJobFailures(limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.query(`SELECT status FROM jobs
 WHERE status IN ('completed','failed') ORDER BY ended_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	failures := 0
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return 0, err
		}
		if status != "failed" {
			break
		}
		failures++
	}
	return failures, rows.Err()
}
