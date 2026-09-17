package core

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/shiori/internal/model"
	"github.com/sirupsen/logrus"
)

// ParseNetscapeBookmarks parses the contents of a Netscape Bookmark HTML
// file (as exported by Firefox, Chrome and other browsers) and returns the
// parsed bookmarks. When generateTag is set, the name of the folder that
// contains each bookmark is added as a tag.
//
// Entries with invalid URLs are skipped and reported in the returned
// errors slice. Duplicated URLs inside the file are skipped silently, as
// they cannot happen in a well-formed export.
func ParseNetscapeBookmarks(htmlContent string, generateTag bool, logger *logrus.Logger) ([]model.BookmarkDTO, []string) {
	var (
		bookmarks []model.BookmarkDTO
		errs      []string
	)

	mapURL := make(map[string]struct{})

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, []string{err.Error()}
	}

	doc.Find("dt>a").Each(func(_ int, a *goquery.Selection) {
		// Get related elements
		dt := a.Parent()
		dl := dt.Parent()
		h3 := dl.Parent().Find("h3").First()

		// Get metadata
		title := a.Text()
		url, _ := a.Attr("href")
		strTags, _ := a.Attr("tags")

		dateStr, fieldExists := a.Attr("last_modified")
		if !fieldExists {
			dateStr, _ = a.Attr("add_date")
		}

		// Bookmarks exported by browsers store unix timestamps; format them
		// as UTC so results don't depend on the server's local timezone.
		modifiedDate := time.Now().UTC()
		if dateStr != "" {
			modifiedTsInt, err := strconv.Atoi(dateStr)
			if err != nil {
				errs = append(errs, "Skip "+url+": date field is not valid: "+err.Error())
				return
			}

			modifiedDate = time.Unix(int64(modifiedTsInt), 0).UTC()
		}

		// Clean up URL
		url, err = RemoveUTMParams(url)
		if err != nil {
			errs = append(errs, "Skip "+url+": URL is not valid")
			return
		}

		// Make sure title is valid Utf-8
		title = ValidateTitle(title, url)

		// Check if the URL already exists in the file itself
		if _, exist := mapURL[url]; exist {
			return
		}

		// Get bookmark tags
		tags := []model.TagDTO{}
		for _, strTag := range strings.Split(strTags, ",") {
			strTag = strings.Join(strings.Fields(strTag), " ")
			if strTag != "" {
				tags = append(tags, model.TagDTO{
					Tag: model.Tag{Name: strTag},
				})
			}
		}

		// Get category name for this bookmark
		// and add it as tags (if necessary)
		category := strings.Join(strings.Fields(h3.Text()), " ")
		if category != "" && generateTag {
			tags = append(tags, model.TagDTO{
				Tag: model.Tag{Name: category},
			})
		}

		// Add item to list
		bookmark := model.BookmarkDTO{
			URL:        url,
			Title:      title,
			Tags:       tags,
			ModifiedAt: modifiedDate.Format(model.DatabaseDateFormat),
		}

		mapURL[url] = struct{}{}
		bookmarks = append(bookmarks, bookmark)
	})

	return bookmarks, errs
}

// ValidateTitle normalizes and validates a bookmark title, using the
// URL as fallback when the title is empty or contains invalid UTF-8.
func ValidateTitle(title, fallback string) string {
	// Normalize spaces before we begin
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")

	// If at this point title already empty, just uses fallback
	if title == "" {
		return fallback
	}

	// Check if it's already valid UTF-8 string
	if valid := utf8.ValidString(title); valid {
		return title
	}

	// Remove invalid runes to get the valid UTF-8 title
	fixUtf := func(r rune) rune {
		if r == utf8.RuneError {
			return -1
		}
		return r
	}
	validUtf := strings.Map(fixUtf, title)

	// If it's empty use fallback string
	validUtf = strings.TrimSpace(validUtf)
	if validUtf == "" {
		return fallback
	}

	return validUtf
}
