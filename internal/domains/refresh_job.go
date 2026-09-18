package domains

import (
	"context"
	sqldatabase "database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/gofrs/uuid/v5"
)

const (
	// refreshConcurrency is the number of bookmarks processed in parallel.
	refreshConcurrency = 3
	// refreshBookmarkTimeout limits the time spent on a single bookmark.
	refreshBookmarkTimeout = 60 * time.Second
	// refreshProgressIdsChunk guards against pathological pending lists.
	refreshProgressSaveEvery = 5
)

// ErrRefreshJobAlreadyRunning is returned when starting a job while another is running.
var ErrRefreshJobAlreadyRunning = model.ErrRefreshJobAlreadyRunning

// RefreshJobDomain manages the persistent background job that refreshes
// bookmark titles, excerpts and thumbnails.
type RefreshJobDomain struct {
	deps model.Dependencies

	mu        sync.Mutex
	cancel    context.CancelFunc
	running   bool
	processor model.RefreshBookmarkFunc
}

// NewRefreshJobDomain creates the domain. The processor defaults to
// UpdateBookmarkCache and can be swapped for tests via SetProcessor.
func NewRefreshJobDomain(deps model.Dependencies) *RefreshJobDomain {
	return &RefreshJobDomain{
		deps: deps,
	}
}

// SetProcessor overrides the bookmark processor (tests).
func (d *RefreshJobDomain) SetProcessor(p model.RefreshBookmarkFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.processor = p
}

func (d *RefreshJobDomain) getProcessor() model.RefreshBookmarkFunc {
	if d.processor != nil {
		return d.processor
	}
	return func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		return d.deps.Domains().Bookmarks().UpdateBookmarkCache(ctx, bookmark, false, true)
	}
}

// GetProgress returns the durable progress of the last refresh job.
// Returns a zero-value DTO and no error when no job has ever been run.
func (d *RefreshJobDomain) GetProgress(ctx context.Context) (*model.RefreshJobDTO, error) {
	job, err := d.deps.Database().GetLastRefreshJob(ctx)
	if err != nil {
		if err == sqldatabase.ErrNoRows {
			return &model.RefreshJobDTO{Status: model.RefreshJobStatus("")}, nil
		}
		return nil, fmt.Errorf("failed to read refresh job: %w", err)
	}
	return job, nil
}

// StartJob refreshes all bookmarks in the database in the background.
// Returns the job ID immediately; progress must be polled from the
// database. Refuses to start when another job is already running.
func (d *RefreshJobDomain) StartJob(ctx context.Context) (string, error) {
	bookmarks, err := d.deps.Database().GetBookmarks(ctx, model.DBGetBookmarksOptions{
		OrderMethod: model.DefaultOrder,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list all bookmarks: %w", err)
	}

	ids := make([]int64, 0, len(bookmarks))
	for _, b := range bookmarks {
		ids = append(ids, int64(b.ID))
	}
	return d.startJobWithIDs(ctx, ids)
}

// StartJobWithIDs launches a refresh job for a specific set of bookmark IDs.
func (d *RefreshJobDomain) StartJobWithIDs(ctx context.Context, ids []int64) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("no bookmarks to refresh")
	}
	return d.startJobWithIDs(ctx, ids)
}

func (d *RefreshJobDomain) startJobWithIDs(ctx context.Context, ids []int64) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return "", ErrRefreshJobAlreadyRunning
	}

	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("failed to create job id: %w", err)
	}
	jobID := id.String()

	now := time.Now().UTC().Format(time.RFC3339)
	job := model.RefreshJobDTO{
		ID:         jobID,
		Status:     model.RefreshJobStatusRunning,
		Total:      int64(len(ids)),
		PendingIDs: encodeIDs(ids),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := d.deps.Database().CreateRefreshJob(ctx, job); err != nil {
		return "", fmt.Errorf("failed to create refresh job: %w", err)
	}

	// mark running before releasing the lock
	d.running = true
	jobCtx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	go d.runJob(jobCtx, jobID)
	return jobID, nil
}

// encodeIDs serialises a list of bookmark IDs for the pending_ids column.
func encodeIDs(ids []int64) string {
	data, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// decodeIDs deserialises the pending_ids column.
func decodeIDs(raw string) []int64 {
	if raw == "" {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// EncodeRefreshJobIDs serialises a list of pending bookmark IDs
// (exported for tests and DB fixtures).
func EncodeRefreshJobIDs(ids []int64) string {
	return encodeIDs(ids)
}

// ResumePendingJobs checks the database for a refresh job that was
// interrupted by a restart and resumes it if found.
func (d *RefreshJobDomain) ResumePendingJobs(ctx context.Context) {
	d.mu.Lock()
	alreadyRunning := d.running
	d.mu.Unlock()
	if alreadyRunning {
		return
	}

	job, err := d.deps.Database().GetLastRefreshJob(ctx)
	if err != nil {
		return // no job record, nothing to resume
	}

	if job.Status != model.RefreshJobStatusRunning || len(decodeIDs(job.PendingIDs)) == 0 {
		d.deps.Logger().Debug("no pending refresh jobs to resume")
		return
	}

	d.deps.Logger().WithField("job_id", job.ID).Info("resuming pending refresh job")

	d.mu.Lock()
	d.running = true
	jobCtx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.mu.Unlock()

	go d.runJob(jobCtx, job.ID)
}

// runJob processes the pending bookmarks for the job with the given ID.
func (d *RefreshJobDomain) runJob(ctx context.Context, jobID string) {
	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	processor := d.getProcessor()

	// Reload pending work from the database so progress survives restarts.
	job, err := d.deps.Database().GetLastRefreshJob(ctx)
	if err != nil {
		d.deps.Logger().WithError(err).Error("failed to load refresh job")
		return
	}

	if string(job.Status) != string(model.RefreshJobStatusRunning) {
		return
	}

	pending := decodeIDs(job.PendingIDs)
	processed := job.Completed
	updated := job.Updated
	failed := job.Failed
	saveCounter := 0

	var workerWG sync.WaitGroup
	var stateMu sync.Mutex
	remaining := pending

	for {
		if ctx.Err() != nil {
			break
		}
		if len(remaining) == 0 {
			break
		}
		batch := remaining
		remaining = nil

		// Feed bookmarks to workers
		type result struct {
			id    int64
			job   model.RefreshJobDTO
			found bool
		}
		batchCh := make(chan int64, len(batch))
		for _, id := range batch {
			batchCh <- id
		}
		close(batchCh)

		for i := 0; i < refreshConcurrency; i++ {
			workerWG.Add(1)
			go func() {
				defer workerWG.Done()
				for id := range batchCh {
					if ctx.Err() != nil {
						return
					}

					bCtx, cancel := context.WithTimeout(ctx, refreshBookmarkTimeout)
					bookmark, err := d.deps.Domains().Bookmarks().GetBookmark(bCtx, model.DBID(id))
					if err != nil {
						cancel()
						stateMu.Lock()
						failed++
						stateMu.Unlock()
						continue
					}

					_ = processor
					updatedBook, err := processor(bCtx, *bookmark)
					cancel()

					stateMu.Lock()
					processed++
					if err != nil {
						failed++
					} else {
						// Fetch a fresh copy to preserve tags and save them as
						// the processor returns only the processed bookmark.
						fresh, ferr := d.deps.Domains().Bookmarks().GetBookmark(context.Background(), model.DBID(id))
						if ferr == nil {
							m := *updatedBook
							m.Tags = fresh.Tags
							_ = m
							_, err = d.deps.Database().SaveBookmarks(ctx, false, *updatedBook)
							if err != nil {
								d.deps.Logger().WithError(err).Error("failed to save refreshed bookmark")
								failed++
							} else {
								updated++
							}
						}
					}
					stateMu.Unlock()
				}
			}()
		}

		workerWG.Wait()

		saveCounter++
		if saveCounter%refreshProgressSaveEvery == 0 || len(remaining) == 0 {
			d.saveProgress(ctx, job.ID, processed, updated, failed, remaining)
		}
	}

	d.saveProgress(ctx, job.ID, processed, updated, failed, remaining)
}

func (d *RefreshJobDomain) saveProgress(ctx context.Context, jobID string, processed, updated, failed int64, remaining []int64) {
	job, err := d.deps.Database().GetLastRefreshJob(ctx)
	if err != nil {
		return
	}
	if job.ID != jobID {
		return
	}

	job.Completed = int64(len(remaining)) // updated to reflect current pending work
	status := model.RefreshJobStatusRunning
	if len(remaining) == 0 || ctx.Err() != nil {
		status = model.RefreshJobStatusDone
	}
	job.Status = status
	job.Completed = processed
	job.Updated = updated
	job.Failed = failed
	job.PendingIDs = encodeIDs(remaining)

	if err := d.deps.Database().UpdateRefreshJob(ctx, *job); err != nil {
		d.deps.Logger().WithError(err).Error("failed to persist refresh job progress")
	}
}
