package domains

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-shiori/shiori/internal/core"
	"github.com/go-shiori/shiori/internal/model"
)

// ParseImport parses the contents of a Netscape Bookmark HTML file
// (Firefox, Chrome, etc.), returning the parsed bookmarks plus a list
// of non-fatal errors for entries that were skipped during parsing.
func (d *BookmarksDomain) ParseImport(htmlContent string, generateTag bool) model.ImportedBookmarks {
	bookmarks, errs := core.ParseNetscapeBookmarks(htmlContent, generateTag, d.deps.Logger())

	return model.ImportedBookmarks{
		Bookmarks: bookmarks,
		Errors:    errs,
	}
}

// ImportBookmarks saves previously parsed bookmarks into the database,
// skipping the ones whose URL already exists. Parsed bookmarks are only
// removed from the result when *both* create is true and saving
// succeeds; otherwise the caller keeps the full list to retry or
// inspect.
func (d *BookmarksDomain) ImportBookmarks(ctx context.Context, parsed model.ImportedBookmarks, create bool) (*model.ImportResult, error) {
	result := &model.ImportResult{
		Errors: parsed.Errors,
	}

	// Filter out bookmarks whose URL already exists in the database
	saveable := make([]model.BookmarkDTO, 0, len(parsed.Bookmarks))
	for _, bookmark := range parsed.Bookmarks {
		_, exist, err := d.deps.Database().GetBookmark(ctx, 0, bookmark.URL)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		if exist {
			result.Skipped++
			continue
		}

		saveable = append(saveable, bookmark)
	}

	if !create || len(saveable) == 0 {
		result.Bookmarks = saveable
		return result, nil
	}

	saved, err := d.deps.Database().SaveBookmarks(ctx, true, saveable...)
	if err != nil {
		return nil, err
	}

	result.Created = int64(len(saved))
	result.Bookmarks = saved

	return result, nil
}
