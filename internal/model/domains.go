package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/go-shiori/warc"
	"github.com/spf13/afero"
)

type BookmarksDomain interface {
	HasEbook(b *BookmarkDTO) bool
	HasArchive(b *BookmarkDTO) bool
	HasThumbnail(b *BookmarkDTO) bool
	GetBookmark(ctx context.Context, id DBID) (*BookmarkDTO, error)
	GetBookmarks(ctx context.Context, ids []int) ([]BookmarkDTO, error)
	UpdateBookmarkCache(ctx context.Context, bookmark BookmarkDTO, keepMetadata bool, skipExist bool) (*BookmarkDTO, error)
	BulkUpdateBookmarkTags(ctx context.Context, bookmarkIDs []int, tagIDs []int) error
	AddTagToBookmark(ctx context.Context, bookmarkID int, tagID int) error
	RemoveTagFromBookmark(ctx context.Context, bookmarkID int, tagID int) error
	BookmarkExists(ctx context.Context, id int) (bool, error)
	ParseImport(htmlContent string, generateTag bool) ImportedBookmarks
	ImportBookmarks(ctx context.Context, parsed ImportedBookmarks, create bool) (*ImportResult, error)
}

// RefreshBookmarkFunc processes a bookmark (fetches page, extracts
// title/excerpt/thumbnail) and returns the updated bookmark.
type RefreshBookmarkFunc func(ctx context.Context, bookmark BookmarkDTO) (*BookmarkDTO, error)

// RefreshJobStatus is the status of a persistent refresh job.
type RefreshJobStatus string

const (
	RefreshJobStatusRunning   RefreshJobStatus = "running"
	RefreshJobStatusDone      RefreshJobStatus = "done"
	RefreshJobStatusCancelled RefreshJobStatus = "cancelled"
)

// RefreshJobDTO tracks a persistent background refresh job across process restarts.
type RefreshJobDTO struct {
	ID         string           `json:"id"`
	Status     RefreshJobStatus `json:"status"`
	Total      int64            `json:"total"`
	Completed  int64            `json:"completed"`
	Updated    int64            `json:"updated"`
	Failed     int64            `json:"failed"`
	PendingIDs string           `json:"-"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
}

// ErrRefreshJobAlreadyRunning is returned when starting a job while another is running.
var ErrRefreshJobAlreadyRunning = errors.New("a refresh job is already running")

// RefreshJobDomain manages the persistent background refresh job.
type RefreshJobDomain interface {
	StartJob(ctx context.Context) (string, error)
	StartJobWithIDs(ctx context.Context, ids []int64) (string, error)
	GetProgress(ctx context.Context) (*RefreshJobDTO, error)
	SetProcessor(processor RefreshBookmarkFunc)
	ResumePendingJobs(ctx context.Context)
}

// ImportedBookmarks holds the result of parsing an import file
type ImportedBookmarks struct {
	Bookmarks []BookmarkDTO
	Errors    []string
}

// ImportResult holds the statistics of an import operation
type ImportResult struct {
	Created   int64
	Skipped   int64
	Errors    []string
	Bookmarks []BookmarkDTO
}

type AuthDomain interface {
	CheckToken(ctx context.Context, userJWT string) (*AccountDTO, error)
	GetAccountFromCredentials(ctx context.Context, username, password string) (*AccountDTO, error)
	CreateTokenForAccount(account *AccountDTO, expiration time.Time) (string, error)
}

type AccountsDomain interface {
	ListAccounts(ctx context.Context) ([]AccountDTO, error)
	GetAccountByUsername(ctx context.Context, username string) (*AccountDTO, error)
	CreateAccount(ctx context.Context, account AccountDTO) (*AccountDTO, error)
	UpdateAccount(ctx context.Context, account AccountDTO) (*AccountDTO, error)
	DeleteAccount(ctx context.Context, id int) error
}

type ArchiverDomain interface {
	DownloadBookmarkArchive(book BookmarkDTO) (*BookmarkDTO, error)
	GetBookmarkArchive(book *BookmarkDTO) (*warc.Archive, error)
}

type StorageDomain interface {
	Stat(name string) (fs.FileInfo, error)
	FS() afero.Fs
	FileExists(path string) bool
	DirExists(path string) bool
	WriteData(dst string, data []byte) error
	WriteFile(dst string, src *os.File) error
}

type TagsDomain interface {
	ListTags(ctx context.Context, opts ListTagsOptions) ([]TagDTO, error)
	CreateTag(ctx context.Context, tag TagDTO) (TagDTO, error)
	GetTag(ctx context.Context, id int) (TagDTO, error)
	UpdateTag(ctx context.Context, tag TagDTO) (TagDTO, error)
	DeleteTag(ctx context.Context, id int) error
	TagExists(ctx context.Context, id int) (bool, error)
}
