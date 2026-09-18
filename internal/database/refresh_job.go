package database

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/go-shiori/shiori/internal/model"
)

const refreshJobColumns = "id, status, total, completed, updated, failed, pending_ids, created_at, updated_at"

// scanRefreshJob scans a refresh_job row into a DTO.
func scanRefreshJob(row interface {
	Scan(dest ...any) error
}) (*model.RefreshJobDTO, error) {
	var job model.RefreshJobDTO
	err := row.Scan(&job.ID, &job.Status, &job.Total, &job.Completed, &job.Updated,
		&job.Failed, &job.PendingIDs, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// CreateRefreshJob inserts a new refresh job row.
func (db *SQLiteDatabase) CreateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return createRefreshJobRow(ctx, db.WriterDB(), job)
}

// CreateRefreshJob inserts a new refresh job row.
func (db *PGDatabase) CreateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return createRefreshJobRow(ctx, db.WriterDB(), job)
}

// CreateRefreshJob inserts a new refresh job row.
func (db *MySQLDatabase) CreateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return createRefreshJobRow(ctx, db.WriterDB(), job)
}

func createRefreshJobRow(ctx context.Context, w *sqlx.DB, job model.RefreshJobDTO) error {
	query := "INSERT INTO refresh_job (" + refreshJobColumns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	args := []any{job.ID, string(job.Status), job.Total, job.Completed, job.Updated,
		job.Failed, job.PendingIDs, job.CreatedAt, job.UpdatedAt}
	query = w.Rebind(query)
	_, err := w.ExecContext(ctx, query, args...)
	return err
}

// UpdateRefreshJob updates an existing refresh job row.
func (db *SQLiteDatabase) UpdateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return updateRefreshJobRow(ctx, db.WriterDB(), job)
}

// UpdateRefreshJob updates an existing refresh job row.
func (db *PGDatabase) UpdateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return updateRefreshJobRow(ctx, db.WriterDB(), job)
}

// UpdateRefreshJob updates an existing refresh job row.
func (db *MySQLDatabase) UpdateRefreshJob(ctx context.Context, job model.RefreshJobDTO) error {
	return updateRefreshJobRow(ctx, db.WriterDB(), job)
}

func updateRefreshJobRow(ctx context.Context, w *sqlx.DB, job model.RefreshJobDTO) error {
	query := `UPDATE refresh_job SET status = ?, total = ?, completed = ?, updated = ?,
		failed = ?, pending_ids = ?, updated_at = ? WHERE id = ?`
	args := []any{string(job.Status), job.Total, job.Completed, job.Updated,
		job.Failed, job.PendingIDs, time.Now().UTC(), job.ID}
	query = w.Rebind(query)
	_, err := w.ExecContext(ctx, query, args...)
	return err
}

// GetLastRefreshJob returns the most recent refresh job, or nil if there is none.
func (db *SQLiteDatabase) GetLastRefreshJob(ctx context.Context) (*model.RefreshJobDTO, error) {
	return getLastRefreshJobRow(ctx, db.ReaderDB())
}

// GetLastRefreshJob returns the most recent refresh job, or nil if there is none.
func (db *PGDatabase) GetLastRefreshJob(ctx context.Context) (*model.RefreshJobDTO, error) {
	return getLastRefreshJobRow(ctx, db.ReaderDB())
}

// GetLastRefreshJob returns the most recent refresh job, or nil if there is none.
func (db *MySQLDatabase) GetLastRefreshJob(ctx context.Context) (*model.RefreshJobDTO, error) {
	return getLastRefreshJobRow(ctx, db.ReaderDB())
}

func getLastRefreshJobRow(ctx context.Context, r *sqlx.DB) (*model.RefreshJobDTO, error) {
	query := "SELECT " + refreshJobColumns + " FROM refresh_job ORDER BY created_at DESC, id DESC"
	query = r.Rebind(query + " LIMIT 1")
	job, err := scanRefreshJob(r.QueryRowContext(ctx, query))
	if err != nil {
		return nil, err // sql.ErrNoRows means no job exists
	}
	return job, nil
}
