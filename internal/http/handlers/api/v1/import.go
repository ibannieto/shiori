package api_v1

import (
	"io"
	"net/http"

	"github.com/go-shiori/shiori/internal/http/middleware"
	"github.com/go-shiori/shiori/internal/http/response"
	"github.com/go-shiori/shiori/internal/model"
)

// importMaxFileSize is the maximum accepted size for an uploaded
// bookmark file (10 MB).
const importMaxFileSize = 10 << 20

type importResponse struct {
	Created int64                `json:"created"`
	Skipped int64                `json:"skipped"`
	Errors  []string             `json:"errors"`
	Parsed  []parsedBookmarksDTO `json:"parsed,omitempty"`
}

type parsedBookmarksDTO struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// HandleImportBookmarks imports bookmarks from an uploaded Netscape
// Bookmark HTML file (Firefox, Chrome, ...).
//
//	@Summary	Import bookmarks from HTML file.
//	@Tags		Bookmarks
//	@Security	ApiKeyAuth
//	@Accept		multipart/form-data
//	@Param		file			formData	file	true	"Netscape Bookmark HTML file"
//	@Param		generate_tag	formData	bool	false	"Add bookmark's folder as tag"
//	@Produce	json
//	@Success	200	{object}	importResponse
//	@Failure	400	{object}	nil	"Invalid request"
//	@Failure	403	{object}	nil	"Token not provided/invalid"
//	@Failure	413	{object}	nil	"File too large"
//	@Router		/api/v1/bookmarks/import [post]
func HandleImportBookmarks(deps model.Dependencies, c model.WebContext) {
	if err := middleware.RequireLoggedInUser(deps, c); err != nil {
		response.SendError(c, http.StatusForbidden, err.Error())
		return
	}

	// Limit upload size to avoid unbounded memory usage
	c.Request().Body = http.MaxBytesReader(c.ResponseWriter(), c.Request().Body, importMaxFileSize)

	if err := c.Request().ParseMultipartForm(importMaxFileSize); err != nil {
		response.SendError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	file, _, err := c.Request().FormFile("file")
	if err != nil {
		response.SendError(c, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	// Reject files that claim to be bigger than the limit
	if seeker, ok := file.(io.ReadSeeker); ok {
		if size, err := seeker.Seek(0, io.SeekEnd); err != nil || size > importMaxFileSize {
			response.SendError(c, http.StatusRequestEntityTooLarge, "File too large")
			return
		}
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			response.SendError(c, http.StatusBadRequest, "Failed to read file")
			return
		}
	}

	content, err := io.ReadAll(io.LimitReader(file, importMaxFileSize+1))
	if err != nil {
		response.SendError(c, http.StatusBadRequest, "Failed to read file")
		return
	}
	if len(content) > importMaxFileSize {
		response.SendError(c, http.StatusRequestEntityTooLarge, "File too large")
		return
	}

	generateTag := c.Request().FormValue("generate_tag") == "true"

	parsed := deps.Domains().Bookmarks().ParseImport(string(content), generateTag)
	if len(parsed.Bookmarks) == 0 && len(parsed.Errors) == 0 {
		response.SendError(c, http.StatusBadRequest, "No bookmarks found in file")
		return
	}

	result, err := deps.Domains().Bookmarks().ImportBookmarks(c.Request().Context(), parsed, true)
	if err != nil {
		deps.Logger().WithError(err).Error("failed to import bookmarks")
		response.SendError(c, http.StatusInternalServerError, "Failed to import bookmarks")
		return
	}

	dto := importResponse{
		Created: result.Created,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}
	for _, bookmark := range result.Bookmarks {
		dto.Parsed = append(dto.Parsed, parsedBookmarksDTO{
			URL:   bookmark.URL,
			Title: bookmark.Title,
		})
	}

	response.SendJSON(c, http.StatusOK, dto)

	// Kick off a background refresh of titles and thumbnails for all
	// bookmarks. Failures are logged by the job itself and progress is
	// polled by the web interface.
	if _, err := deps.Domains().RefreshJobs().StartJob(c.Request().Context()); err != nil {
		// Already-running or database failure is not fatal for the import.
		deps.Logger().WithError(err).Warn("could not start background refresh job")
	}
}
