package domains_test

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/go-shiori/shiori/internal/testutil"
)

const singleBookmarkHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><A HREF="https://github.com/go-shiori/shiori">Shiori</A>
</DL><p>
`

func TestImportBookmarksCreatesAndSkipsDuplicates(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
	bookmarksDomain := deps.Domains().Bookmarks()

	// First import: created
	result, err := bookmarksDomain.ImportBookmarks(ctx, bookmarksDomain.ParseImport(singleBookmarkHTML, false), true)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Created)
	require.Len(t, result.Bookmarks, 1)

	// Second import: duplicate skipped
	result, err = bookmarksDomain.ImportBookmarks(ctx, bookmarksDomain.ParseImport(singleBookmarkHTML, false), true)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Skipped)
	require.Empty(t, result.Bookmarks)
}

func TestImportBookmarksWithGenerateTag(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
	bookmarksDomain := deps.Domains().Bookmarks()

	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><H3>Dev</H3>
	<DL><p>
		<DT><A HREF="https://github.com/go-shiori/shiori">Shiori</A>
	</DL><p>
</DL><p>
`

	result, err := bookmarksDomain.ImportBookmarks(ctx, bookmarksDomain.ParseImport(html, true), true)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Created)
	require.Len(t, result.Bookmarks, 1)
	require.Len(t, result.Bookmarks[0].Tags, 1)
	require.Equal(t, "Dev", result.Bookmarks[0].Tags[0].Name)
}

func TestImportBookmarksParseInvalidURL(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
	bookmarksDomain := deps.Domains().Bookmarks()

	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><A HREF=":">Bad</A>
</DL><p>
`

	parsed := bookmarksDomain.ParseImport(html, false)
	require.Len(t, parsed.Errors, 1)
	require.Empty(t, parsed.Bookmarks)
}
