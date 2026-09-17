# API v1

> ℹ️ **This is the documentation for the new API. This API is still in development and though the finished endpoints should not change please consider that breaking changes may occur once its properly released. If you are looking for the current API, please [see here](./API.md).**

The new API is an ongoing effort to migrate the current API to a more modern and standard API.

The main goals of this new API are:
- Ease of development
- Use of a [modern framework](https://gin-gonic.com)
- Use of a [standard API specification](https://swagger.io/specification/)
- Self-documented API using [Swag](https://github.com/swaggo/swag)
- Improved authentication and sessions using [JWT](https://jwt.io)
- Deduplicate code between the webserver and the API by refactoring the logic into domains
- Improve testability by using interfaces and dependency injection

The current status of this new API can be checked [here](https://github.com/go-shiori/shiori/issues/640).

Since the API is self-docummented, you can check the API documentation by [running the server locally](./Contribute.md#running-the-server-locally) and visiting the [`/swagger/index.html` endpoint](http://localhost:8080/swagger/index.html).

## Import bookmarks from the web interface

`POST /api/v1/bookmarks/import` imports bookmarks from an uploaded Netscape Bookmark HTML file (as exported by Firefox, Chrome and other browsers). It accepts `multipart/form-data` with:

- `file` (required): the Netscape Bookmark HTML file (up to 10 MB).
- `generate_tag` (optional, `true`/`false`): add each bookmark's parent folder name as a tag, same as the `--generate-tag` flag of the CLI `import` command.

The endpoint requires authentication and returns the number of created and skipped bookmarks plus a list of non-fatal parsing errors:

```json
{
  "ok": true,
  "message": { "created": 1, "skipped": 1, "errors": null, "parsed": [{ "title": "One", "url": "https://example.com/one" }] }
}
```

Bookmarks whose URL already exists are skipped, mirroring the behavior of `shiori import`.
