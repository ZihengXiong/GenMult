package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dialect string

const (
	dialectPostgres dialect = "postgres"
	dialectSQLite   dialect = "sqlite"
)

type SQLStore struct {
	dialect dialect
	pg      *pgxpool.Pool
	sq      *sql.DB
}

func NewPostgresSQLStore(pool *pgxpool.Pool) (*SQLStore, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &SQLStore{dialect: dialectPostgres, pg: pool}, nil
}

func NewSQLiteSQLStore(db *sql.DB) (*SQLStore, error) {
	if db == nil {
		return nil, errors.New("sqlite db is required")
	}
	return &SQLStore{dialect: dialectSQLite, sq: db}, nil
}

// EnsureSchema creates orchestrator tables if they do not exist. This is
// intended for tests and local development only — production deployments
// should use the migration files (db/postgres/migrations, db/sqlite/migrations).
func (s *SQLStore) EnsureSchema(ctx context.Context) error {
	return s.ensureSchema(ctx)
}

func (s *SQLStore) ensureSchema(ctx context.Context) error {
	for _, ddl := range s.schemaDDL() {
		if err := s.exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) schemaDDL() []string {
	if s.dialect == dialectPostgres {
		return []string{
			`CREATE TABLE IF NOT EXISTS agent_hub_runs (
			  id UUID PRIMARY KEY,
			  room_id UUID NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
			  trigger_message_id TEXT NOT NULL DEFAULT '',
			  objective TEXT NOT NULL,
			  status TEXT NOT NULL,
			  planner_version TEXT NOT NULL DEFAULT '',
			  created_by TEXT NOT NULL DEFAULT '',
			  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			  created_at_ms BIGINT NOT NULL,
			  updated_at_ms BIGINT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_hub_runs_room_updated ON agent_hub_runs(room_id, updated_at_ms DESC);`,
			`CREATE TABLE IF NOT EXISTS agent_hub_tasks (
			  id UUID PRIMARY KEY,
			  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
			  parent_task_id UUID,
			  title TEXT NOT NULL,
			  description TEXT NOT NULL,
			  assigned_agent_id TEXT NOT NULL DEFAULT '',
			  provider_name TEXT NOT NULL DEFAULT '',
			  priority INTEGER NOT NULL DEFAULT 0,
			  status TEXT NOT NULL,
			  timeout_ms BIGINT NOT NULL DEFAULT 0,
			  max_retries INTEGER NOT NULL DEFAULT 0,
			  attempt_count INTEGER NOT NULL DEFAULT 0,
			  idempotency_key TEXT NOT NULL DEFAULT '',
			  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			  created_at_ms BIGINT NOT NULL,
			  updated_at_ms BIGINT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_hub_tasks_run_priority ON agent_hub_tasks(run_id, status, priority DESC, created_at_ms ASC);`,
			`CREATE TABLE IF NOT EXISTS agent_hub_task_deps (
			  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
			  task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
			  depends_on_task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
			  PRIMARY KEY (task_id, depends_on_task_id)
			);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_hub_task_deps_run ON agent_hub_task_deps(run_id, task_id);`,
			`CREATE TABLE IF NOT EXISTS agent_hub_task_attempts (
			  id UUID PRIMARY KEY,
			  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
			  task_id UUID NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
			  attempt_no INTEGER NOT NULL,
			  provider_name TEXT NOT NULL,
			  agent_id TEXT NOT NULL DEFAULT '',
			  status TEXT NOT NULL,
			  input_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			  output_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			  error_message TEXT NOT NULL DEFAULT '',
			  retryable BOOLEAN NOT NULL DEFAULT false,
			  started_at_ms BIGINT NOT NULL,
			  finished_at_ms BIGINT,
			  idempotency_key TEXT NOT NULL,
			  UNIQUE (task_id, attempt_no)
			);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_hub_task_attempts_run ON agent_hub_task_attempts(run_id, task_id, attempt_no DESC);`,
			`CREATE TABLE IF NOT EXISTS agent_hub_run_events (
			  id UUID PRIMARY KEY,
			  run_id UUID NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
			  task_id UUID,
			  seq BIGINT NOT NULL,
			  type TEXT NOT NULL,
			  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			  created_at_ms BIGINT NOT NULL,
			  UNIQUE (run_id, seq)
			);`,
			`CREATE INDEX IF NOT EXISTS idx_agent_hub_run_events_seq ON agent_hub_run_events(run_id, seq ASC);`,
		}
	}
	return []string{
		`CREATE TABLE IF NOT EXISTS agent_hub_runs (
		  id TEXT PRIMARY KEY,
		  room_id TEXT NOT NULL REFERENCES agent_hub_rooms(id) ON DELETE CASCADE,
		  trigger_message_id TEXT NOT NULL DEFAULT '',
		  objective TEXT NOT NULL,
		  status TEXT NOT NULL,
		  planner_version TEXT NOT NULL DEFAULT '',
		  created_by TEXT NOT NULL DEFAULT '',
		  metadata TEXT NOT NULL DEFAULT '{}',
		  created_at_ms INTEGER NOT NULL,
		  updated_at_ms INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hub_runs_room_updated ON agent_hub_runs(room_id, updated_at_ms DESC);`,
		`CREATE TABLE IF NOT EXISTS agent_hub_tasks (
		  id TEXT PRIMARY KEY,
		  run_id TEXT NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
		  parent_task_id TEXT,
		  title TEXT NOT NULL,
		  description TEXT NOT NULL,
		  assigned_agent_id TEXT NOT NULL DEFAULT '',
		  provider_name TEXT NOT NULL DEFAULT '',
		  priority INTEGER NOT NULL DEFAULT 0,
		  status TEXT NOT NULL,
		  timeout_ms INTEGER NOT NULL DEFAULT 0,
		  max_retries INTEGER NOT NULL DEFAULT 0,
		  attempt_count INTEGER NOT NULL DEFAULT 0,
		  idempotency_key TEXT NOT NULL DEFAULT '',
		  metadata TEXT NOT NULL DEFAULT '{}',
		  created_at_ms INTEGER NOT NULL,
		  updated_at_ms INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hub_tasks_run_priority ON agent_hub_tasks(run_id, status, priority DESC, created_at_ms ASC);`,
		`CREATE TABLE IF NOT EXISTS agent_hub_task_deps (
		  run_id TEXT NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
		  task_id TEXT NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
		  depends_on_task_id TEXT NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
		  PRIMARY KEY (task_id, depends_on_task_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hub_task_deps_run ON agent_hub_task_deps(run_id, task_id);`,
		`CREATE TABLE IF NOT EXISTS agent_hub_task_attempts (
		  id TEXT PRIMARY KEY,
		  run_id TEXT NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
		  task_id TEXT NOT NULL REFERENCES agent_hub_tasks(id) ON DELETE CASCADE,
		  attempt_no INTEGER NOT NULL,
		  provider_name TEXT NOT NULL,
		  agent_id TEXT NOT NULL DEFAULT '',
		  status TEXT NOT NULL,
		  input_payload TEXT NOT NULL DEFAULT '{}',
		  output_payload TEXT NOT NULL DEFAULT '{}',
		  error_message TEXT NOT NULL DEFAULT '',
		  retryable INTEGER NOT NULL DEFAULT 0,
		  started_at_ms INTEGER NOT NULL,
		  finished_at_ms INTEGER,
		  idempotency_key TEXT NOT NULL,
		  UNIQUE (task_id, attempt_no)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hub_task_attempts_run ON agent_hub_task_attempts(run_id, task_id, attempt_no DESC);`,
		`CREATE TABLE IF NOT EXISTS agent_hub_run_events (
		  id TEXT PRIMARY KEY,
		  run_id TEXT NOT NULL REFERENCES agent_hub_runs(id) ON DELETE CASCADE,
		  task_id TEXT,
		  seq INTEGER NOT NULL,
		  type TEXT NOT NULL,
		  payload TEXT NOT NULL DEFAULT '{}',
		  created_at_ms INTEGER NOT NULL,
		  UNIQUE (run_id, seq)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hub_run_events_seq ON agent_hub_run_events(run_id, seq ASC);`,
	}
}

func (s *SQLStore) CreateRun(ctx context.Context, run Run) (Run, error) {
	metadata := marshalJSON(run.Metadata)
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `INSERT INTO agent_hub_runs (id, room_id, trigger_message_id, objective, status, planner_version, created_by, metadata, created_at_ms, updated_at_ms)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			run.ID, run.RoomID, run.TriggerMessageID, run.Objective, string(run.Status), run.PlannerVersion, run.CreatedBy, metadata, toMillis(run.CreatedAt), toMillis(run.UpdatedAt))
		if err != nil {
			return Run{}, err
		}
		return run, nil
	}
	_, err := s.sq.ExecContext(ctx, `INSERT INTO agent_hub_runs (id, room_id, trigger_message_id, objective, status, planner_version, created_by, metadata, created_at_ms, updated_at_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.RoomID, run.TriggerMessageID, run.Objective, string(run.Status), run.PlannerVersion, run.CreatedBy, string(metadata), toMillis(run.CreatedAt), toMillis(run.UpdatedAt))
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *SQLStore) GetRun(ctx context.Context, runID string) (Run, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Run{}, ErrInvalidInput
	}
	var (
		run                      Run
		status                   string
		metadataRaw              []byte
		createdAtMS, updatedAtMS int64
	)
	if s.dialect == dialectPostgres {
		err := s.pg.QueryRow(ctx, `SELECT id, room_id, trigger_message_id, objective, status, planner_version, created_by, metadata, created_at_ms, updated_at_ms FROM agent_hub_runs WHERE id=$1`, runID).
			Scan(&run.ID, &run.RoomID, &run.TriggerMessageID, &run.Objective, &status, &run.PlannerVersion, &run.CreatedBy, &metadataRaw, &createdAtMS, &updatedAtMS)
		if err != nil {
			return Run{}, mapNotFound(err)
		}
	} else {
		var metadataText string
		err := s.sq.QueryRowContext(ctx, `SELECT id, room_id, trigger_message_id, objective, status, planner_version, created_by, metadata, created_at_ms, updated_at_ms FROM agent_hub_runs WHERE id=?`, runID).
			Scan(&run.ID, &run.RoomID, &run.TriggerMessageID, &run.Objective, &status, &run.PlannerVersion, &run.CreatedBy, &metadataText, &createdAtMS, &updatedAtMS)
		if err != nil {
			return Run{}, mapNotFound(err)
		}
		metadataRaw = []byte(metadataText)
	}
	run.Status = RunStatus(status)
	run.Metadata = unmarshalJSONMap(metadataRaw)
	run.CreatedAt = fromMillis(createdAtMS)
	run.UpdatedAt = fromMillis(updatedAtMS)
	return run, nil
}

func (s *SQLStore) UpdateRunStatus(ctx context.Context, runID string, status RunStatus) (Run, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if !canTransitionRun(run.Status, status) {
		return Run{}, ErrInvalidTransition
	}
	if run.Status == status {
		return run, nil
	}
	nowMS := toMillis(time.Now().UTC())
	var affected int64
	if s.dialect == dialectPostgres {
		tag, execErr := s.pg.Exec(ctx, `UPDATE agent_hub_runs SET status=$1, updated_at_ms=$2 WHERE id=$3 AND status=$4`, string(status), nowMS, run.ID, string(run.Status))
		if execErr != nil {
			return Run{}, execErr
		}
		affected = tag.RowsAffected()
	} else {
		res, execErr := s.sq.ExecContext(ctx, `UPDATE agent_hub_runs SET status=?, updated_at_ms=? WHERE id=? AND status=?`, string(status), nowMS, run.ID, string(run.Status))
		if execErr != nil {
			return Run{}, execErr
		}
		affected, _ = res.RowsAffected()
	}
	if affected == 0 {
		return Run{}, ErrInvalidTransition
	}
	return s.GetRun(ctx, run.ID)
}

func (s *SQLStore) ListRunsByStatus(ctx context.Context, statuses ...RunStatus) ([]Run, error) {
	where := ""
	args := make([]any, 0)
	if len(statuses) > 0 {
		parts := make([]string, 0, len(statuses))
		for i, status := range statuses {
			parts = append(parts, s.placeholder(i+1))
			args = append(args, string(status))
		}
		where = " WHERE status IN (" + strings.Join(parts, ",") + ")"
	}
	query := `SELECT id FROM agent_hub_runs` + where + ` ORDER BY created_at_ms ASC`
	ids := make([]string, 0)
	if s.dialect == dialectPostgres {
		rows, err := s.pg.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
	} else {
		rows, err := s.sq.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
	}
	out := make([]Run, 0, len(ids))
	for _, id := range ids {
		run, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (s *SQLStore) CreateTasks(ctx context.Context, runID string, drafts []TaskDraft) ([]Task, []TaskDependency, error) {
	if strings.TrimSpace(runID) == "" || len(drafts) == 0 {
		return nil, nil, ErrInvalidInput
	}
	clientToTask := make(map[string]Task, len(drafts))
	now := time.Now().UTC()
	for _, draft := range drafts {
		clientKey := strings.TrimSpace(draft.ClientKey)
		if clientKey == "" {
			return nil, nil, ErrInvalidInput
		}
		if _, ok := clientToTask[clientKey]; ok {
			return nil, nil, ErrDuplicateClientKey
		}
		task := Task{
			ID:              newID(),
			RunID:           runID,
			Title:           strings.TrimSpace(draft.Title),
			Description:     strings.TrimSpace(draft.Description),
			AssignedAgentID: strings.TrimSpace(draft.AssignedAgentID),
			ProviderName:    strings.TrimSpace(draft.ProviderName),
			Priority:        draft.Priority,
			Status:          TaskStatusPending,
			Timeout:         draft.Timeout,
			MaxRetries:      draft.MaxRetries,
			AttemptCount:    0,
			Metadata:        cloneMap(draft.Metadata),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		clientToTask[clientKey] = task
	}

	for _, draft := range drafts {
		task := clientToTask[strings.TrimSpace(draft.ClientKey)]
		if parentKey := strings.TrimSpace(draft.ParentClientKey); parentKey != "" {
			parent, ok := clientToTask[parentKey]
			if !ok {
				return nil, nil, ErrInvalidInput
			}
			task.ParentTaskID = parent.ID
			clientToTask[strings.TrimSpace(draft.ClientKey)] = task
		}
	}

	tasks := make([]Task, 0, len(drafts))
	for _, draft := range drafts {
		task := clientToTask[strings.TrimSpace(draft.ClientKey)]
		if err := s.insertTask(ctx, task); err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, task)
	}

	deps := make([]TaskDependency, 0)
	for _, draft := range drafts {
		task := clientToTask[strings.TrimSpace(draft.ClientKey)]
		for _, depKey := range draft.DependsOn {
			depTask, ok := clientToTask[strings.TrimSpace(depKey)]
			if !ok {
				return nil, nil, ErrInvalidInput
			}
			dep := TaskDependency{RunID: runID, TaskID: task.ID, DependsOnTaskID: depTask.ID}
			if err := s.insertDependency(ctx, dep); err != nil {
				return nil, nil, err
			}
			deps = append(deps, dep)
		}
	}
	return tasks, deps, nil
}

func (s *SQLStore) insertTask(ctx context.Context, task Task) error {
	metadata := marshalJSON(task.Metadata)
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `INSERT INTO agent_hub_tasks (id, run_id, parent_task_id, title, description, assigned_agent_id, provider_name, priority, status, timeout_ms, max_retries, attempt_count, idempotency_key, metadata, created_at_ms, updated_at_ms)
			VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			task.ID, task.RunID, task.ParentTaskID, task.Title, task.Description, task.AssignedAgentID, task.ProviderName, task.Priority, string(task.Status), int64(task.Timeout/time.Millisecond), task.MaxRetries, task.AttemptCount, task.IdempotencyKey, metadata, toMillis(task.CreatedAt), toMillis(task.UpdatedAt))
		return err
	}
	_, err := s.sq.ExecContext(ctx, `INSERT INTO agent_hub_tasks (id, run_id, parent_task_id, title, description, assigned_agent_id, provider_name, priority, status, timeout_ms, max_retries, attempt_count, idempotency_key, metadata, created_at_ms, updated_at_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		task.ID, task.RunID, emptyToNil(task.ParentTaskID), task.Title, task.Description, task.AssignedAgentID, task.ProviderName, task.Priority, string(task.Status), int64(task.Timeout/time.Millisecond), task.MaxRetries, task.AttemptCount, task.IdempotencyKey, string(metadata), toMillis(task.CreatedAt), toMillis(task.UpdatedAt))
	return err
}

func (s *SQLStore) insertDependency(ctx context.Context, dep TaskDependency) error {
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `INSERT INTO agent_hub_task_deps (run_id, task_id, depends_on_task_id) VALUES ($1,$2,$3) ON CONFLICT (task_id, depends_on_task_id) DO NOTHING`, dep.RunID, dep.TaskID, dep.DependsOnTaskID)
		return err
	}
	_, err := s.sq.ExecContext(ctx, `INSERT OR IGNORE INTO agent_hub_task_deps (run_id, task_id, depends_on_task_id) VALUES (?,?,?)`, dep.RunID, dep.TaskID, dep.DependsOnTaskID)
	return err
}

func (s *SQLStore) ListTasks(ctx context.Context, runID string) ([]Task, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, ErrInvalidInput
	}
	query := `SELECT id, run_id, COALESCE(parent_task_id,''), title, description, assigned_agent_id, provider_name, priority, status, timeout_ms, max_retries, attempt_count, idempotency_key, metadata, created_at_ms, updated_at_ms FROM agent_hub_tasks WHERE run_id=` + s.placeholder(1) + ` ORDER BY created_at_ms ASC`
	args := []any{runID}
	out := make([]Task, 0)
	if s.dialect == dialectPostgres {
		rows, err := s.pg.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			task, err := scanTaskPG(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, task)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	}
	rows, err := s.sq.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		task, err := scanTaskSQLite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *SQLStore) UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus) (Task, error) {
	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	if !canTransitionTask(task.Status, status) {
		return Task{}, ErrInvalidTransition
	}
	if task.Status == status {
		return task, nil
	}
	updatedAtMS := toMillis(time.Now().UTC())
	var affected int64
	if s.dialect == dialectPostgres {
		tag, execErr := s.pg.Exec(ctx, `UPDATE agent_hub_tasks SET status=$1, updated_at_ms=$2 WHERE id=$3 AND status=$4`, string(status), updatedAtMS, task.ID, string(task.Status))
		if execErr != nil {
			return Task{}, execErr
		}
		affected = tag.RowsAffected()
	} else {
		res, execErr := s.sq.ExecContext(ctx, `UPDATE agent_hub_tasks SET status=?, updated_at_ms=? WHERE id=? AND status=?`, string(status), updatedAtMS, task.ID, string(task.Status))
		if execErr != nil {
			return Task{}, execErr
		}
		affected, _ = res.RowsAffected()
	}
	if affected == 0 {
		return Task{}, ErrInvalidTransition
	}
	return s.getTask(ctx, task.ID)
}

func (s *SQLStore) IncrementTaskAttempt(ctx context.Context, taskID string) (Task, error) {
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `UPDATE agent_hub_tasks SET attempt_count = attempt_count + 1, updated_at_ms=$1 WHERE id=$2`, toMillis(time.Now().UTC()), taskID)
		if err != nil {
			return Task{}, err
		}
	} else {
		_, err := s.sq.ExecContext(ctx, `UPDATE agent_hub_tasks SET attempt_count = attempt_count + 1, updated_at_ms=? WHERE id=?`, toMillis(time.Now().UTC()), taskID)
		if err != nil {
			return Task{}, err
		}
	}
	return s.getTask(ctx, taskID)
}

func (s *SQLStore) getTask(ctx context.Context, taskID string) (Task, error) {
	taskID = strings.TrimSpace(taskID)
	query := `SELECT id, run_id, COALESCE(parent_task_id,''), title, description, assigned_agent_id, provider_name, priority, status, timeout_ms, max_retries, attempt_count, idempotency_key, metadata, created_at_ms, updated_at_ms FROM agent_hub_tasks WHERE id=` + s.placeholder(1)
	if s.dialect == dialectPostgres {
		row := s.pg.QueryRow(ctx, query, taskID)
		return scanTaskPG(row)
	}
	row := s.sq.QueryRowContext(ctx, query, taskID)
	return scanTaskSQLite(row)
}

func (s *SQLStore) ListDependencies(ctx context.Context, runID string) ([]TaskDependency, error) {
	runID = strings.TrimSpace(runID)
	query := `SELECT run_id, task_id, depends_on_task_id FROM agent_hub_task_deps WHERE run_id=` + s.placeholder(1) + ` ORDER BY task_id ASC`
	out := make([]TaskDependency, 0)
	if s.dialect == dialectPostgres {
		rows, err := s.pg.Query(ctx, query, runID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var dep TaskDependency
			if err := rows.Scan(&dep.RunID, &dep.TaskID, &dep.DependsOnTaskID); err != nil {
				return nil, err
			}
			out = append(out, dep)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	}
	rows, err := s.sq.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dep TaskDependency
		if err := rows.Scan(&dep.RunID, &dep.TaskID, &dep.DependsOnTaskID); err != nil {
			return nil, err
		}
		out = append(out, dep)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *SQLStore) CreateAttempt(ctx context.Context, attempt TaskAttempt) (TaskAttempt, error) {
	input := marshalJSON(attempt.InputPayload)
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `INSERT INTO agent_hub_task_attempts (id, run_id, task_id, attempt_no, provider_name, agent_id, status, input_payload, output_payload, error_message, retryable, started_at_ms, finished_at_ms, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'{}'::jsonb,'',$9,$10,NULL,$11)`, attempt.ID, attempt.RunID, attempt.TaskID, attempt.AttemptNo, attempt.ProviderName, attempt.AgentID, string(attempt.Status), input, attempt.Retryable, toMillis(attempt.StartedAt), attempt.IdempotencyKey)
		if err != nil {
			return TaskAttempt{}, err
		}
		return attempt, nil
	}
	_, err := s.sq.ExecContext(ctx, `INSERT INTO agent_hub_task_attempts (id, run_id, task_id, attempt_no, provider_name, agent_id, status, input_payload, output_payload, error_message, retryable, started_at_ms, finished_at_ms, idempotency_key)
		VALUES (?,?,?,?,?,?,?,?,'{}','',?,?,NULL,?)`, attempt.ID, attempt.RunID, attempt.TaskID, attempt.AttemptNo, attempt.ProviderName, attempt.AgentID, string(attempt.Status), string(input), boolToInt(attempt.Retryable), toMillis(attempt.StartedAt), attempt.IdempotencyKey)
	if err != nil {
		return TaskAttempt{}, err
	}
	return attempt, nil
}

func (s *SQLStore) CompleteAttempt(ctx context.Context, attemptID string, status AttemptStatus, output map[string]any, errorMessage string, retryable bool) (TaskAttempt, error) {
	finishedAt := toMillis(time.Now().UTC())
	outputRaw := marshalJSON(output)
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, `UPDATE agent_hub_task_attempts SET status=$1, output_payload=$2, error_message=$3, retryable=$4, finished_at_ms=$5 WHERE id=$6`, string(status), outputRaw, errorMessage, retryable, finishedAt, attemptID)
		if err != nil {
			return TaskAttempt{}, err
		}
	} else {
		_, err := s.sq.ExecContext(ctx, `UPDATE agent_hub_task_attempts SET status=?, output_payload=?, error_message=?, retryable=?, finished_at_ms=? WHERE id=?`, string(status), string(outputRaw), errorMessage, boolToInt(retryable), finishedAt, attemptID)
		if err != nil {
			return TaskAttempt{}, err
		}
	}
	attempt, err := s.getAttempt(ctx, attemptID)
	if err != nil {
		return TaskAttempt{}, err
	}
	return attempt, nil
}

func (s *SQLStore) ListAttempts(ctx context.Context, runID string) ([]TaskAttempt, error) {
	query := `SELECT id, task_id, run_id, attempt_no, provider_name, agent_id, status, input_payload, output_payload, error_message, retryable, started_at_ms, finished_at_ms, idempotency_key FROM agent_hub_task_attempts WHERE run_id=` + s.placeholder(1) + ` ORDER BY started_at_ms ASC, attempt_no ASC`
	out := make([]TaskAttempt, 0)
	if s.dialect == dialectPostgres {
		rows, err := s.pg.Query(ctx, query, runID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			attempt, err := scanAttemptPG(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, attempt)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	}
	rows, err := s.sq.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		attempt, err := scanAttemptSQLite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, attempt)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *SQLStore) getAttempt(ctx context.Context, attemptID string) (TaskAttempt, error) {
	query := `SELECT id, task_id, run_id, attempt_no, provider_name, agent_id, status, input_payload, output_payload, error_message, retryable, started_at_ms, finished_at_ms, idempotency_key FROM agent_hub_task_attempts WHERE id=` + s.placeholder(1)
	if s.dialect == dialectPostgres {
		row := s.pg.QueryRow(ctx, query, attemptID)
		return scanAttemptPG(row)
	}
	row := s.sq.QueryRowContext(ctx, query, attemptID)
	return scanAttemptSQLite(row)
}

func (s *SQLStore) AppendEvent(ctx context.Context, event RunEvent) (RunEvent, error) {
	payload := marshalJSON(event.Payload)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	createdAtMS := toMillis(event.CreatedAt)

	// Retry on UNIQUE(run_id, seq) conflict — two concurrent INSERTs can
	// read the same MAX(seq) and collide.
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			event.ID = uuid.NewString()
		}
		var err error
		if s.dialect == dialectPostgres {
			err = s.pg.QueryRow(ctx, `INSERT INTO agent_hub_run_events (id, run_id, task_id, seq, type, payload, created_at_ms)
				VALUES ($1,$2,NULLIF($3,''),COALESCE((SELECT MAX(seq)+1 FROM agent_hub_run_events WHERE run_id=$2),1),$4,$5,$6)
				RETURNING seq`, event.ID, event.RunID, event.TaskID, string(event.Type), payload, createdAtMS).Scan(&event.Seq)
		} else {
			err = s.sq.QueryRowContext(ctx, `INSERT INTO agent_hub_run_events (id, run_id, task_id, seq, type, payload, created_at_ms)
				VALUES (?, ?, NULLIF(?,''), COALESCE((SELECT MAX(seq)+1 FROM agent_hub_run_events WHERE run_id=?),1), ?, ?, ?)
				RETURNING seq`, event.ID, event.RunID, event.TaskID, event.RunID, string(event.Type), string(payload), createdAtMS).Scan(&event.Seq)
		}
		if err == nil {
			event.Payload = cloneMap(event.Payload)
			return event, nil
		}
		if !isUniqueViolation(err) {
			return RunEvent{}, err
		}
	}
	return RunEvent{}, fmt.Errorf("append event: seq conflict after %d retries", maxRetries)
}

func (s *SQLStore) ListEvents(ctx context.Context, runID string, afterSeq int64, limit int32) ([]RunEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `SELECT id, run_id, COALESCE(task_id,''), seq, type, payload, created_at_ms FROM agent_hub_run_events WHERE run_id=` + s.placeholder(1) + ` AND seq > ` + s.placeholder(2) + ` ORDER BY seq ASC LIMIT ` + s.placeholder(3)
	out := make([]RunEvent, 0)
	if s.dialect == dialectPostgres {
		rows, err := s.pg.Query(ctx, query, runID, afterSeq, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanEventPG(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, event)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return out, nil
	}
	rows, err := s.sq.QueryContext(ctx, query, runID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		event, err := scanEventSQLite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *SQLStore) exec(ctx context.Context, query string, args ...any) error {
	if s.dialect == dialectPostgres {
		_, err := s.pg.Exec(ctx, query, args...)
		return err
	}
	_, err := s.sq.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLStore) placeholder(i int) string {
	if s.dialect == dialectPostgres {
		return fmt.Sprintf("$%d", i)
	}
	return "?"
}

type scanner interface{ Scan(dest ...any) error }

func scanTaskPG(s scanner) (Task, error) {
	var (
		task                                Task
		status                              string
		metadataRaw                         []byte
		timeoutMS, createdAtMS, updatedAtMS int64
	)
	if err := s.Scan(&task.ID, &task.RunID, &task.ParentTaskID, &task.Title, &task.Description, &task.AssignedAgentID, &task.ProviderName, &task.Priority, &status, &timeoutMS, &task.MaxRetries, &task.AttemptCount, &task.IdempotencyKey, &metadataRaw, &createdAtMS, &updatedAtMS); err != nil {
		return Task{}, mapNotFound(err)
	}
	task.Status = TaskStatus(status)
	task.Timeout = time.Duration(timeoutMS) * time.Millisecond
	task.Metadata = unmarshalJSONMap(metadataRaw)
	task.CreatedAt = fromMillis(createdAtMS)
	task.UpdatedAt = fromMillis(updatedAtMS)
	return task, nil
}

func scanTaskSQLite(s scanner) (Task, error) {
	var (
		task                                Task
		status                              string
		metadataText                        string
		timeoutMS, createdAtMS, updatedAtMS int64
	)
	if err := s.Scan(&task.ID, &task.RunID, &task.ParentTaskID, &task.Title, &task.Description, &task.AssignedAgentID, &task.ProviderName, &task.Priority, &status, &timeoutMS, &task.MaxRetries, &task.AttemptCount, &task.IdempotencyKey, &metadataText, &createdAtMS, &updatedAtMS); err != nil {
		return Task{}, mapNotFound(err)
	}
	task.Status = TaskStatus(status)
	task.Timeout = time.Duration(timeoutMS) * time.Millisecond
	task.Metadata = unmarshalJSONMap([]byte(metadataText))
	task.CreatedAt = fromMillis(createdAtMS)
	task.UpdatedAt = fromMillis(updatedAtMS)
	return task, nil
}

func scanAttemptPG(s scanner) (TaskAttempt, error) {
	var (
		attempt             TaskAttempt
		status              string
		inputRaw, outputRaw []byte
		startedAtMS         int64
		finishedAtMS        sql.NullInt64
	)
	if err := s.Scan(&attempt.ID, &attempt.TaskID, &attempt.RunID, &attempt.AttemptNo, &attempt.ProviderName, &attempt.AgentID, &status, &inputRaw, &outputRaw, &attempt.ErrorMessage, &attempt.Retryable, &startedAtMS, &finishedAtMS, &attempt.IdempotencyKey); err != nil {
		return TaskAttempt{}, mapNotFound(err)
	}
	attempt.Status = AttemptStatus(status)
	attempt.InputPayload = unmarshalJSONMap(inputRaw)
	attempt.OutputPayload = unmarshalJSONMap(outputRaw)
	attempt.StartedAt = fromMillis(startedAtMS)
	if finishedAtMS.Valid {
		finished := fromMillis(finishedAtMS.Int64)
		attempt.FinishedAt = &finished
	}
	return attempt, nil
}

func scanAttemptSQLite(s scanner) (TaskAttempt, error) {
	var (
		attempt                       TaskAttempt
		status, inputText, outputText string
		startedAtMS                   int64
		finishedAtMS                  sql.NullInt64
		retryableInt                  int64
	)
	if err := s.Scan(&attempt.ID, &attempt.TaskID, &attempt.RunID, &attempt.AttemptNo, &attempt.ProviderName, &attempt.AgentID, &status, &inputText, &outputText, &attempt.ErrorMessage, &retryableInt, &startedAtMS, &finishedAtMS, &attempt.IdempotencyKey); err != nil {
		return TaskAttempt{}, mapNotFound(err)
	}
	attempt.Status = AttemptStatus(status)
	attempt.InputPayload = unmarshalJSONMap([]byte(inputText))
	attempt.OutputPayload = unmarshalJSONMap([]byte(outputText))
	attempt.Retryable = retryableInt != 0
	attempt.StartedAt = fromMillis(startedAtMS)
	if finishedAtMS.Valid {
		finished := fromMillis(finishedAtMS.Int64)
		attempt.FinishedAt = &finished
	}
	return attempt, nil
}

func scanEventPG(s scanner) (RunEvent, error) {
	var (
		e           RunEvent
		eType       string
		payloadRaw  []byte
		createdAtMS int64
	)
	if err := s.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Seq, &eType, &payloadRaw, &createdAtMS); err != nil {
		return RunEvent{}, mapNotFound(err)
	}
	e.Type = EventType(eType)
	e.Payload = unmarshalJSONMap(payloadRaw)
	e.CreatedAt = fromMillis(createdAtMS)
	return e, nil
}

func scanEventSQLite(s scanner) (RunEvent, error) {
	var (
		e                  RunEvent
		eType, payloadText string
		createdAtMS        int64
	)
	if err := s.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Seq, &eType, &payloadText, &createdAtMS); err != nil {
		return RunEvent{}, mapNotFound(err)
	}
	e.Type = EventType(eType)
	e.Payload = unmarshalJSONMap([]byte(payloadText))
	e.CreatedAt = fromMillis(createdAtMS)
	return e, nil
}

func marshalJSON(value map[string]any) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func unmarshalJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func toMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}

func fromMillis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func mapNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if strings.Contains(strings.ToLower(err.Error()), "no rows") {
		return ErrNotFound
	}
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Postgres: "duplicate key value violates unique constraint"
	// SQLite:   "UNIQUE constraint failed"
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate key")
}

func newID() string {
	return uuid.NewString()
}
