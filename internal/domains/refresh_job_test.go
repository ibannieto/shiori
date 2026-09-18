package domains_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/go-shiori/shiori/internal/domains"
	"github.com/go-shiori/shiori/internal/model"
	"github.com/go-shiori/shiori/internal/testutil"
)

// refreshBookmark is a unique-URL bookmark to seed the database.
func refreshBookmark(url string) model.BookmarkDTO {
	return model.BookmarkDTO{URL: url, Title: "placeholder"}
}

// waitUntil polls the condition until it holds or the timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("waitUntil: condition not met before timeout")
}

func TestRefreshJobRunsInBackgroundAndSaves(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

	// Seed two bookmarks
	b1, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/a"))
	require.NoError(t, err)
	b2, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/b"))
	require.NoError(t, err)

	var mu sync.Mutex
	var processed []int64
	processor := func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		mu.Lock()
		defer mu.Unlock()
		processed = append(processed, int64(bookmark.ID))
		updatedBook := bookmark
		updatedBook.Title = "Refreshed title"
		return &updatedBook, nil
	}

	refreshDomain := domains.NewRefreshJobDomain(deps)
	refreshDomain.SetProcessor(processor)

	jobID, err := refreshDomain.StartJob(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, jobID)

	// Wait until finished
	waitUntil(t, 3*time.Second, func() bool {
		progress, err := refreshDomain.GetProgress(ctx)
		require.NoError(t, err)
		return progress.Status == model.RefreshJobStatusDone
	})

	progress, err := refreshDomain.GetProgress(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), progress.Total)
	require.Equal(t, int64(2), progress.Completed)
	require.Equal(t, int64(2), progress.Updated)
	require.Equal(t, int64(0), progress.Failed)

	// Titles must be persisted
	saved, err := deps.Database().GetBookmarks(ctx, model.DBGetBookmarksOptions{
		IDs: []int{int(b1[0].ID), int(b2[0].ID)},
	})
	require.NoError(t, err)
	require.Equal(t, "Refreshed title", saved[0].Title)
	require.Equal(t, "Refreshed title", saved[1].Title)
}

func TestRefreshJobCountsFailures(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

	b1, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/a"))
	require.NoError(t, err)
	id := int(b1[0].ID)

	processor := func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		if int64(bookmark.ID) == int64(id) {
			return nil, fmt.Errorf("simulated failure")
		}
		updatedBook := bookmark
		return &updatedBook, nil
	}

	refreshDomain := domains.NewRefreshJobDomain(deps)
	refreshDomain.SetProcessor(processor)

	_, err = refreshDomain.StartJob(ctx)
	require.NoError(t, err)

	waitUntil(t, 3*time.Second, func() bool {
		progress, err := refreshDomain.GetProgress(ctx)
		require.NoError(t, err)
		return progress.Status == model.RefreshJobStatusDone
	})

	progress, _ := refreshDomain.GetProgress(ctx)
	require.Equal(t, int64(1), progress.Failed)
}

func TestRefreshJobPreventsOverlap(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

	_, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/a"))
	require.NoError(t, err)

	blocked := make(chan struct{})
	var mu sync.Mutex
	var calls int

	refreshDomain := domains.NewRefreshJobDomain(deps)
	refreshDomain.SetProcessor(func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-blocked
		return nil, fmt.Errorf("blocked")
	})

	_, err = refreshDomain.StartJob(ctx)
	require.NoError(t, err)

	// While the first job is running, a second start must be refused
	_, err = refreshDomain.StartJob(ctx)
	require.Error(t, err)

	close(blocked)
	waitUntil(t, 3*time.Second, func() bool {
		progress, _ := refreshDomain.GetProgress(ctx)
		return progress.Status == model.RefreshJobStatusDone
	})
	require.Equal(t, 1, func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}())
}

func TestRefreshJobResumesAfterRestart(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

	// Simulate an interrupted job: a persisted record in running state
	// with pending work, as it would be left behind by a crashed process.
	created, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/resume"))
	require.NoError(t, err)
	require.Len(t, created, 1)

	// Simulate: one bookmark was already processed before interruption
	interrupted := model.RefreshJobDTO{
		ID:         "interrupted-job",
		Status:     model.RefreshJobStatusRunning,
		Total:      1,
		Completed:  0,
		PendingIDs: domains.EncodeRefreshJobIDs([]int64{int64(created[0].ID)}),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	require.NoError(t, deps.Database().CreateRefreshJob(ctx, interrupted))

	var refreshed bool
	var mu sync.Mutex
	refreshDomain := domains.NewRefreshJobDomain(deps)
	refreshDomain.SetProcessor(func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		mu.Lock()
		refreshed = true
		mu.Unlock()
		updatedBook := bookmark
		updatedBook.Title = "Resumed title"
		return &updatedBook, nil
	})

	refreshDomain.ResumePendingJobs(ctx)

	waitUntil(t, 3*time.Second, func() bool {
		progress, _ := refreshDomain.GetProgress(ctx)
		return progress.Status == model.RefreshJobStatusDone
	})

	mu.Lock()
	require.True(t, refreshed)
	mu.Unlock()
}

func TestRefreshJobPersistsProgressToDatabase(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

	created, err := deps.Database().SaveBookmarks(ctx, true, refreshBookmark("https://example.com/a"))
	require.NoError(t, err)
	require.Len(t, created, 1)

	refreshDomain := domains.NewRefreshJobDomain(deps)
	refreshDomain.SetProcessor(func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
		updatedBook := bookmark
		updatedBook.Title = "Refreshed"
		return &updatedBook, nil
	})

	_, err = refreshDomain.StartJob(ctx)
	require.NoError(t, err)

	waitUntil(t, 3*time.Second, func() bool {
		progress, _ := refreshDomain.GetProgress(ctx)
		return progress.Status == model.RefreshJobStatusDone
	})

	rec, err := deps.Database().GetLastRefreshJob(ctx)
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.Equal(t, model.RefreshJobStatusDone, rec.Status)
	require.Equal(t, int64(1), rec.Total)
	require.Equal(t, int64(1), rec.Completed)
	require.Equal(t, int64(1), rec.Updated)
}
