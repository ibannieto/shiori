package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/go-shiori/shiori/internal/model"
	"github.com/spf13/cobra"
)

func importCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import source-file",
		Short: "Import bookmarks from HTML file in Netscape Bookmark format",
		Args:  cobra.ExactArgs(1),
		Run:   importHandler,
	}

	cmd.Flags().BoolP("generate-tag", "t", false, "Auto generate tag from bookmark's category")

	return cmd
}

func importHandler(cmd *cobra.Command, args []string) {
	_, deps := initShiori(cmd.Context(), cmd)

	// Parse flags
	generateTag := cmd.Flags().Changed("generate-tag")

	// If user doesn't specify, ask if tag need to be generated
	if !generateTag {
		var submit string
		fmt.Print("Add parents folder as tag? (y/N): ")
		fmt.Scanln(&submit)

		generateTag = submit == "y"
	}

	// Open bookmark's file
	srcFile, err := os.Open(args[0])
	if err != nil {
		cError.Printf("Failed to open %s: %v\n", args[0], err)
		os.Exit(1)
	}
	defer srcFile.Close()

	content, err := io.ReadAll(srcFile)
	if err != nil {
		cError.Printf("Failed to read %s: %v\n", args[0], err)
		os.Exit(1)
	}

	parsed := deps.Domains().Bookmarks().ParseImport(string(content), generateTag)
	for _, errMsg := range parsed.Errors {
		cError.Println(errMsg)
	}

	result, err := deps.Domains().Bookmarks().ImportBookmarks(cmd.Context(), parsed, true)
	if err != nil {
		cError.Printf("Failed to save bookmarks: %v\n", err)
		os.Exit(1)
	}

	// Print skipped bookmarks from the database
	for _, bookmark := range parsed.Bookmarks {
		if !containsBookmark(result.Bookmarks, bookmark.URL) {
			cError.Printf("Skip %s: URL already exists\n", bookmark.URL)
		}
	}

	// Print imported bookmark
	fmt.Println()
	printBookmarks(result.Bookmarks...)
}

func containsBookmark(bookmarks []model.BookmarkDTO, url string) bool {
	for _, bookmark := range bookmarks {
		if bookmark.URL == url {
			return true
		}
	}
	return false
}
