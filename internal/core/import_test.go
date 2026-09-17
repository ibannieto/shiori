package core

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const validNetscapeHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
	<DT><H3>Development</H3>
	<DL><p>
		<DT><A HREF="https://github.com/go-shiori/shiori" ADD_DATE="1670000000" LAST_MODIFIED="1670000001">Shiori</A>
		<DT><A HREF="https://golang.org" TAGS="go,programming" ADD_DATE="1670000002">Go</A>
	</DL><p>
</DT>
</DL><p>
`

func TestParseNetscapeBookmarks(t *testing.T) {
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks(validNetscapeHTML, true, logger)

	require.Empty(t, errs)
	require.Len(t, bookmarks, 2)

	// First bookmark: folder tag generated when requested
	require.Equal(t, "https://github.com/go-shiori/shiori", bookmarks[0].URL)
	require.Equal(t, "Shiori", bookmarks[0].Title)
	require.Len(t, bookmarks[0].Tags, 1)
	require.Equal(t, "Development", bookmarks[0].Tags[0].Name)

	// Second bookmark: keeps its own TAGS attribute, plus folder tag
	require.Equal(t, "https://golang.org", bookmarks[1].URL)
	require.Equal(t, "Go", bookmarks[1].Title)
	require.Len(t, bookmarks[1].Tags, 3)
	require.Equal(t, "go", bookmarks[1].Tags[0].Name)
	require.Equal(t, "programming", bookmarks[1].Tags[1].Name)
}

func TestParseNetscapeBookmarksWithModifiedDate(t *testing.T) {
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks(validNetscapeHTML, false, logger)

	require.Empty(t, errs)
	require.Len(t, bookmarks, 2)
	// LAST_MODIFIED=1670000001 takes precedence over ADD_DATE (UTC)
	require.Equal(t, "2022-12-02 16:53", bookmarks[0].ModifiedAt[:16])
}

func TestParseNetscapeBookmarksWithoutFolderTag(t *testing.T) {
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks(validNetscapeHTML, false, logger)

	require.Empty(t, errs)
	require.Len(t, bookmarks, 2)
	require.Len(t, bookmarks[0].Tags, 0)
}

func TestParseNetscapeBookmarksInvalidURL(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><A HREF=":">Bad</A>
</DL><p>
`
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks(html, false, logger)

	require.Empty(t, bookmarks)
	require.Len(t, errs, 1)
}

func TestParseNetscapeBookmarksValidDuplicateURL(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><A HREF="https://github.com/go-shiori/shiori">Shiori</A>
	<DT><A HREF="https://github.com/go-shiori/shiori">Shiori again</A>
</DL><p>
`
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks(html, false, logger)

	require.Len(t, bookmarks, 1)
	require.Empty(t, errs)
}

func TestParseNetscapeBookmarksEmpty(t *testing.T) {
	logger := logrus.New()
	bookmarks, errs := ParseNetscapeBookmarks("", false, logger)

	require.Empty(t, bookmarks)
	require.Empty(t, errs)
}
