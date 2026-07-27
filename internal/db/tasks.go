package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskCompleted = "completed"
	TaskFailed    = "failed"
	TaskDead      = "dead"
	TaskCanceled  = "canceled"
)

type QueuedTask struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Options     map[string]any `json:"options,omitempty"`
	Priority    int            `json:"priority"`
	Status      string         `json:"status"`
	Attempts    int            `json:"attempts"`
	MaxAttempts int            `json:"max_attempts"`
	AvailableAt string         `json:"available_at"`
	LeaseUntil  string         `json:"lease_until,omitempty"`
	WorkerID    string         `json:"worker_id,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	StartedAt   string         `json:"started_at,omitempty"`
	EndedAt     string         `json:"ended_at,omitempty"`
}

var ErrQueueFull = errors.New("durable task queue is full")

func (s *Store) EnqueueTask(task *QueuedTask, maxPending int) error {
	if s == nil || task == nil {
		return fmt.Errorf("task is required")
	}
	if task.ID == "" {
		task.ID = taskID()
	}
	if task.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = 3
	}
	if task.Status == "" {
		task.Status = TaskQueued
	}
	now := fmtTime(time.Now())
	if task.AvailableAt == "" {
		task.AvailableAt = now
	}
	task.CreatedAt, task.UpdatedAt = now, now
	options, err := json.Marshal(task.Options)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if maxPending > 0 {
		var pending int
		if err := tx.QueryRow(s.rebind(`SELECT COUNT(*) FROM task_queue WHERE status IN ('queued','running')`)).Scan(&pending); err != nil {
			_ = tx.Rollback()
			return err
		}
		if pending >= maxPending {
			_ = tx.Rollback()
			return ErrQueueFull
		}
	}
	_, err = tx.Exec(s.rebind(`INSERT INTO task_queue(
 id,type,options_json,priority,status,attempts,max_attempts,available_at,created_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?)`), task.ID, task.Type, string(options), task.Priority, task.Status, 0,
		task.MaxAttempts, task.AvailableAt, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) LeaseTask(ctx context.Context, workerID string, lease time.Duration) (*QueuedTask, error) {
	if s == nil {
		return nil, sql.ErrNoRows
	}
	if workerID == "" {
		return nil, fmt.Errorf("worker id is required")
	}
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	now := fmtTime(time.Now())
	leaseUntil := fmtTime(time.Now().Add(lease))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err := requeueExpiredTx(s, tx, now); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	selectQuery := `SELECT id FROM task_queue
 WHERE status='queued' AND available_at<=?
 ORDER BY priority DESC,created_at,id LIMIT 1`
	if s.driver == "postgres" {
		selectQuery += ` FOR UPDATE SKIP LOCKED`
	}
	var id string
	if err := tx.QueryRowContext(ctx, s.rebind(selectQuery), now).Scan(&id); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	result, err := tx.ExecContext(ctx, s.rebind(`UPDATE task_queue SET
 status='running',attempts=attempts+1,lease_until=?,worker_id=?,last_error='',
 started_at=COALESCE(started_at,?),updated_at=?
 WHERE id=? AND status='queued'`), leaseUntil, workerID, now, now, id)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		_ = tx.Rollback()
		return nil, sql.ErrNoRows
	}
	task, err := getTaskTx(s, tx, id)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func requeueExpiredTx(s *Store, tx *sql.Tx, now string) error {
	_, err := tx.Exec(s.rebind(`UPDATE task_queue SET
 status=CASE WHEN attempts>=max_attempts THEN 'dead' ELSE 'queued' END,
 available_at=?,lease_until=NULL,worker_id=NULL,
 last_error=CASE WHEN last_error='' THEN 'worker lease expired' ELSE last_error END,
 ended_at=CASE WHEN attempts>=max_attempts THEN ? ELSE ended_at END,
 updated_at=?
 WHERE status='running' AND lease_until<?`), now, now, now, now)
	return err
}

func getTaskTx(s *Store, tx *sql.Tx, id string) (*QueuedTask, error) {
	row := tx.QueryRow(s.rebind(`SELECT id,type,options_json,priority,status,attempts,max_attempts,available_at,
 lease_until,worker_id,last_error,created_at,updated_at,started_at,ended_at FROM task_queue WHERE id=?`), id)
	return scanTask(row)
}

func (s *Store) GetTask(id string) (*QueuedTask, error) {
	if s == nil {
		return nil, sql.ErrNoRows
	}
	return scanTask(s.queryRow(`SELECT id,type,options_json,priority,status,attempts,max_attempts,available_at,
 lease_until,worker_id,last_error,created_at,updated_at,started_at,ended_at FROM task_queue WHERE id=?`, id))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (*QueuedTask, error) {
	var task QueuedTask
	var options string
	var lease, worker, lastErr, started, ended sql.NullString
	if err := row.Scan(&task.ID, &task.Type, &options, &task.Priority, &task.Status, &task.Attempts,
		&task.MaxAttempts, &task.AvailableAt, &lease, &worker, &lastErr, &task.CreatedAt,
		&task.UpdatedAt, &started, &ended); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(options), &task.Options); err != nil {
		return nil, err
	}
	task.LeaseUntil = lease.String
	task.WorkerID = worker.String
	task.LastError = lastErr.String
	task.StartedAt = started.String
	task.EndedAt = ended.String
	return &task, nil
}

func (s *Store) CompleteTask(id, workerID string) error {
	return s.finishTask(id, workerID, TaskCompleted, "")
}

func (s *Store) RenewTaskLease(id, workerID string, lease time.Duration) (bool, error) {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	now := time.Now()
	result, err := s.exec(`UPDATE task_queue SET lease_until=?,updated_at=?
 WHERE id=? AND status='running' AND worker_id=?`,
		fmtTime(now.Add(lease)), fmtTime(now), id, workerID)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected == 1, nil
}

func (s *Store) FailTask(id, workerID, message string, retryBase time.Duration) error {
	if s == nil {
		return nil
	}
	task, err := s.GetTask(id)
	if err != nil {
		return err
	}
	if task.Status != TaskRunning || task.WorkerID != workerID {
		return fmt.Errorf("task %s is not leased by worker", id)
	}
	now := time.Now()
	if task.Attempts >= task.MaxAttempts {
		return s.finishTask(id, workerID, TaskDead, message)
	}
	if retryBase <= 0 {
		retryBase = 5 * time.Second
	}
	delay := time.Duration(math.Pow(2, float64(task.Attempts-1))) * retryBase
	_, err = s.exec(`UPDATE task_queue SET status='queued',available_at=?,lease_until=NULL,worker_id=NULL,
 last_error=?,updated_at=? WHERE id=? AND status='running' AND worker_id=?`,
		fmtTime(now.Add(delay)), message, fmtTime(now), id, workerID)
	return err
}

func (s *Store) finishTask(id, workerID, status, message string) error {
	if s == nil {
		return nil
	}
	now := fmtTime(time.Now())
	result, err := s.exec(`UPDATE task_queue SET status=?,lease_until=NULL,worker_id=NULL,last_error=?,
 updated_at=?,ended_at=? WHERE id=? AND status='running' AND worker_id=?`,
		status, message, now, now, id, workerID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return fmt.Errorf("task %s is not leased by worker", id)
	}
	return nil
}

func (s *Store) CancelTask(id string) error {
	if s == nil {
		return nil
	}
	now := fmtTime(time.Now())
	result, err := s.exec(`UPDATE task_queue SET status='canceled',lease_until=NULL,worker_id=NULL,
 updated_at=?,ended_at=? WHERE id=? AND status IN ('queued','running')`, now, now, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListTasks(limit int, status string) ([]*QueuedTask, error) {
	return s.ListTasksTenant(limit, status, "")
}

func (s *Store) ListTasksTenant(limit int, status, tenant string) ([]*QueuedTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id,type,options_json,priority,status,attempts,max_attempts,available_at,
 lease_until,worker_id,last_error,created_at,updated_at,started_at,ended_at FROM task_queue`
	args := []any{}
	var conditions []string
	if status != "" {
		conditions = append(conditions, `status=?`)
		args = append(args, status)
	}
	if tenant != "" {
		conditions = append(conditions, `id IN (SELECT id FROM jobs WHERE tenant_id=?)`)
		args = append(args, tenantOrDefault(tenant))
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*QueuedTask
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *Store) ListTasksPageTenant(limit int, status, cursor, tenant string) ([]*QueuedTask, error) {
	if limit <= 0 || limit > 101 {
		limit = 31
	}
	query := `SELECT id,type,options_json,priority,status,attempts,max_attempts,available_at,
 lease_until,worker_id,last_error,created_at,updated_at,started_at,ended_at FROM task_queue`
	conditions, args := taskFilters(status, tenant)
	if cursor != "" {
		var created string
		cursorQuery := `SELECT created_at FROM task_queue WHERE id=?`
		cursorArgs := []any{cursor}
		if tenant != "" {
			cursorQuery += ` AND id IN (SELECT id FROM jobs WHERE tenant_id=?)`
			cursorArgs = append(cursorArgs, tenantOrDefault(tenant))
		}
		if err := s.queryRow(cursorQuery, cursorArgs...).Scan(&created); err != nil {
			return nil, err
		}
		conditions = append(conditions, `(created_at<? OR (created_at=? AND id<?))`)
		args = append(args, created, created, cursor)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*QueuedTask
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *Store) CountTasksTenant(status, tenant string) (int, error) {
	conditions, args := taskFilters(status, tenant)
	query := `SELECT COUNT(*) FROM task_queue`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	var count int
	if err := s.queryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func taskFilters(status, tenant string) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	if status != "" {
		conditions = append(conditions, `status=?`)
		args = append(args, status)
	}
	if tenant != "" {
		conditions = append(conditions, `id IN (SELECT id FROM jobs WHERE tenant_id=?)`)
		args = append(args, tenantOrDefault(tenant))
	}
	return conditions, args
}

func (s *Store) QueueStats() (map[string]int64, error) {
	rows, err := s.query(`SELECT status,COUNT(*) FROM task_queue GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{
		TaskQueued: 0, TaskRunning: 0, TaskCompleted: 0, TaskDead: 0, TaskCanceled: 0,
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func taskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
