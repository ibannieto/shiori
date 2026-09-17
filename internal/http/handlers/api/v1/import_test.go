package api_v1

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/go-shiori/shiori/internal/testutil"
)

const importHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
	<DT><A HREF="https://github.com/go-shiori/shiori">Shiori</A>
</DL><p>
`

func newImportMultipartBody(t *testing.T, filename, content string, generateTag bool) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("generate_tag", func() string {
		if generateTag {
			return "true"
		}
		return "false"
	}()))

	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return body, writer.FormDataContentType()
}

func TestHandleImportBookmarks(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	t.Run("requires authentication", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		body, contentType := newImportMultipartBody(t, "bookmarks.html", importHTML, false)

		w := testutil.PerformRequest(
			deps,
			HandleImportBookmarks,
			http.MethodPost,
			"/api/v1/bookmarks/import",
			testutil.WithBodyReader(body),
			testutil.WithHeader("Content-Type", contentType),
		)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing file", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

		w := testutil.PerformRequest(
			deps,
			HandleImportBookmarks,
			http.MethodPost,
			"/api/v1/bookmarks/import",
			testutil.WithFakeUser(),
		)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid HTML content", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		body, contentType := newImportMultipartBody(t, "bookmarks.html", "this is not a bookmark file", false)

		w := testutil.PerformRequest(
			deps,
			HandleImportBookmarks,
			http.MethodPost,
			"/api/v1/bookmarks/import",
			testutil.WithFakeUser(),
			testutil.WithBodyReader(body),
			testutil.WithHeader("Content-Type", contentType),
		)
		// A file without bookmarks is rejected as invalid input
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)
		body, contentType := newImportMultipartBody(t, "bookmarks.html", importHTML, false)

		w := testutil.PerformRequest(
			deps,
			HandleImportBookmarks,
			http.MethodPost,
			"/api/v1/bookmarks/import",
			testutil.WithFakeUser(),
			testutil.WithBodyReader(body),
			testutil.WithHeader("Content-Type", contentType),
		)
		require.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Created int64 `json:"created"`
			Skipped int64 `json:"skipped"`
		}
		require.NoError(t, json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&response))
		require.Equal(t, int64(1), response.Created)
		require.Equal(t, int64(0), response.Skipped)
	})

	// Regression: bufio.Scanner default 64KB limit on the request body
	// must not garble large bookmark exports.
	t.Run("handles large files", func(t *testing.T) {
		_, deps := testutil.GetTestConfigurationAndDependencies(t, ctx, logger)

		large := "<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><p>"
		for i := 0; i < 2000; i++ {
			large += `<DT><A HREF="https://example.com/page/` + strconv.Itoa(i) + `">Page</A>`
		}
		large += "</DL><p>"

		body, contentType := newImportMultipartBody(t, "bookmarks.html", large, false)
		w := testutil.PerformRequest(
			deps,
			HandleImportBookmarks,
			http.MethodPost,
			"/api/v1/bookmarks/import",
			testutil.WithFakeUser(),
			testutil.WithBodyReader(body),
			testutil.WithHeader("Content-Type", contentType),
		)
		require.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Created int64 `json:"created"`
		}
		require.NoError(t, json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&response))
		require.Equal(t, int64(2000), response.Created)
	})
}
