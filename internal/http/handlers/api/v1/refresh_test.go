package api_v1

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/go-shiori/shiori/internal/testutil"
)

func TestHandleRefreshBookmarks(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("requires authentication", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleRefreshBookmarks, http.MethodPost, "/api/v1/bookmarks/refresh")
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("requires admin", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleRefreshBookmarks, http.MethodPost,
			"/api/v1/bookmarks/refresh", testutil.WithFakeUser())
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("starts job", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		// empty database: starts and immediately finishes
		w := testutil.PerformRequest(deps, HandleRefreshBookmarks, http.MethodPost,
			"/api/v1/bookmarks/refresh", testutil.WithFakeAdmin())
		require.Equal(t, http.StatusOK, w.Code)

		var response struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.NotEmpty(t, response.JobID)
	})

	t.Run("refuses concurrent jobs", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

		refreshDomain := deps.Domains().RefreshJobs()
		refreshDomain.SetProcessor(func(ctx context.Context, bookmark model.BookmarkDTO) (*model.BookmarkDTO, error) {
			block := make(chan struct{})
			<-block // block forever; test cleanup ends the process
			return nil, nil
		})

		// Seed a bookmark so the job stays busy
		bookmark := *testutil.GetValidBookmark()
		_, err := deps.Database().SaveBookmarks(ctx, true, bookmark)
		require.NoError(t, err)

		w := testutil.PerformRequest(deps, HandleRefreshBookmarks, http.MethodPost,
			"/api/v1/bookmarks/refresh", testutil.WithFakeAdmin())
		require.Equal(t, http.StatusOK, w.Code)

		w2 := testutil.PerformRequest(deps, HandleRefreshBookmarks, http.MethodPost,
			"/api/v1/bookmarks/refresh", testutil.WithFakeAdmin())
		require.Equal(t, http.StatusConflict, w2.Code)
	})
}

func TestHandleRefreshProgress(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("requires authentication", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleRefreshProgress, http.MethodGet, "/api/v1/bookmarks/refresh/progress")
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("returns empty progress when no job ran", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		w := testutil.PerformRequest(deps, HandleRefreshProgress, http.MethodGet,
			"/api/v1/bookmarks/refresh/progress", testutil.WithFakeUser())
		require.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Status string `json:"status"`
			Total  int64  `json:"total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Empty(t, response.Status)
		require.Equal(t, int64(0), response.Total)
	})
}
