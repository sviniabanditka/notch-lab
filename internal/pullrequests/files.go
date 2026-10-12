package pullrequests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// prFile is one entry of GitHub's list of a pull request's files. Patch is
// empty for a binary file and for one too large for GitHub to diff.
type prFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
}

const (
	filesPerPage = 100
	// maxFiles is where GitHub stops listing a pull request's files.
	maxFiles = 3000
)

const unavailable = "Diff not available"

// files lists a pull request's files page by page. The .diff media type
// is no substitute: GitHub refuses it above 300 files.
func (c *client) files(ctx context.Context, r repo, number int) ([]prFile, error) {
	var all []prFile
	for page := 1; len(all) < maxFiles; page++ {
		var batch []prFile
		path := fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=%d&page=%d", r, number, filesPerPage, page)
		if _, err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < filesPerPage {
			break
		}
	}
	return all[:min(len(all), maxFiles)], nil
}

// unifiedDiff writes files as one git diff, and names the files whose
// change it has no lines for.
func unifiedDiff(files []prFile) (diff string, missing []string) {
	var b strings.Builder
	for _, f := range files {
		old := f.Filename
		if f.PreviousFilename != "" {
			old = f.PreviousFilename
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", old, f.Filename)
		switch f.Status {
		case "added":
			b.WriteString("new file mode 100644\n")
		case "removed":
			b.WriteString("deleted file mode 100644\n")
		case "renamed":
			fmt.Fprintf(&b, "rename from %s\nrename to %s\n", old, f.Filename)
		}
		if f.Patch == "" {
			if f.Changes > 0 || f.Status != "renamed" {
				missing = append(missing, f.Filename)
			}
			continue
		}
		oldPath, newPath := "a/"+old, "b/"+f.Filename
		switch f.Status {
		case "added":
			oldPath = "/dev/null"
		case "removed":
			newPath = "/dev/null"
		}
		fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldPath, newPath)
		b.WriteString(f.Patch)
		if !strings.HasSuffix(f.Patch, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), missing
}
