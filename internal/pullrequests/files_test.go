package pullrequests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestFilesFollowsPagesToTheLastShortOne(t *testing.T) {
	const total = 205
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		if r.URL.Path != "/repos/o/r/pulls/7/files" || r.URL.Query().Get("per_page") != "100" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var batch []prFile
		for i := (page - 1) * 100; i < min(page*100, total); i++ {
			batch = append(batch, prFile{Filename: fmt.Sprintf("f%d", i)})
		}
		json.NewEncoder(w).Encode(batch)
	}))
	defer srv.Close()
	c := newClient(srv.URL)
	c.token = "test"
	files, err := c.files(context.Background(), repo{"o", "r"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != total || files[204].Filename != "f204" {
		t.Fatalf("got %d files, last %q", len(files), files[len(files)-1].Filename)
	}
	if fmt.Sprint(pages) != "[1 2 3]" {
		t.Fatalf("asked pages %v, want [1 2 3]", pages)
	}
}

func TestUnifiedDiffFromPatches(t *testing.T) {
	files := []prFile{
		{Filename: "a.go", Status: "modified", Changes: 2, Patch: "@@ -1 +1 @@\n-x\n+y"},
		{Filename: "new.txt", Status: "added", Changes: 1, Patch: "@@ -0,0 +1 @@\n+hi\n"},
		{Filename: "gone.txt", Status: "removed", Changes: 1, Patch: "@@ -1 +0,0 @@\n-bye"},
		{Filename: "logo.png", Status: "added"},
		{Filename: "huge.json", Status: "modified", Changes: 90000},
		{Filename: "b.go", PreviousFilename: "a2.go", Status: "renamed"},
	}
	diff, missing := unifiedDiff(files)
	want := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-x
+y
diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+hi
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/logo.png b/logo.png
new file mode 100644
diff --git a/huge.json b/huge.json
diff --git a/a2.go b/b.go
rename from a2.go
rename to b.go
`
	if diff != want {
		t.Errorf("diff =\n%s\nwant\n%s", diff, want)
	}
	if fmt.Sprint(missing) != "[logo.png huge.json]" {
		t.Errorf("missing = %v, want [logo.png huge.json]", missing)
	}
}
