package api_v1

import (
	"net/http"

	"github.com/go-shiori/shiori/internal/http/middleware"
	"github.com/go-shiori/shiori/internal/http/response"
	"github.com/go-shiori/shiori/internal/model"
)

type refreshJobResponse struct {
	JobID string `json:"job_id"`
}

// HandleRefreshBookmarks starts a persistent background job that
// refreshes the title, excerpt and thumbnail of every bookmark.
//
//	@Summary	Refresh titles and thumbnails of all bookmarks in background.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Produce	json
//	@Success	200	{object}	refreshJobResponse
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Failure	409	{object}	nil	"Job already running"
//	@Router		/api/v1/bookmarks/refresh [post]
func HandleRefreshBookmarks(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInAdmin(deps, c); err != nil {
		response.SendError(c, http.StatusForbidden, err.Error())
		return
	}

	jobID, err := deps.Domains().RefreshJobs().StartJob(c.Request().Context())
	if err == model.ErrRefreshJobAlreadyRunning {
		response.SendError(c, http.StatusConflict, "A refresh job is already running")
		return
	}
	if err != nil {
		deps.Logger().WithError(err).Error("failed to start refresh job")
		response.SendError(c, http.StatusInternalServerError, "Failed to start refresh job")
		return
	}

	response.SendJSON(c, http.StatusOK, refreshJobResponse{JobID: jobID})
}

// HandleRefreshProgress returns the progress of the last refresh job.
//
//	@Summary	Get refresh job progress.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Produce	json
//	@Success	200	{object}	model.RefreshJobDTO
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Router		/api/v1/bookmarks/refresh/progress [get]
func HandleRefreshProgress(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInUser(deps, c); err != nil {
		response.SendError(c, http.StatusForbidden, err.Error())
		return
	}

	progress, err := deps.Domains().RefreshJobs().GetProgress(c.Request().Context())
	if err != nil {
		deps.Logger().WithError(err).Error("failed to read refresh progress")
		response.SendError(c, http.StatusInternalServerError, "Failed to read refresh progress")
		return
	}

	response.SendJSON(c, http.StatusOK, progress)
}
