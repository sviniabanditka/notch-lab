package filetree

import (
	"slices"
	"testing"
)

func TestSortPutsFoldersFirstInNaturalCaseInsensitiveOrder(t *testing.T) {
	es := []entry{
		{name: "file10.txt"}, {name: "Zeta", isDir: true}, {name: "File2.txt"}, {name: "b.go"},
		{name: "alpha", isDir: true}, {name: ".env"}, {name: "file1.txt"}, {name: "img2", isDir: true},
		{name: "img10", isDir: true}, {name: "Readme.md"}, {name: "readme.MD"},
	}
	sortEntries(es)
	var got []string
	for _, e := range es {
		got = append(got, e.name)
	}
	want := []string{"alpha", "img2", "img10", "Zeta", ".env", "b.go", "file1.txt", "File2.txt", "file10.txt", "Readme.md", "readme.MD"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %q\nwant    %q", got, want)
	}
}
