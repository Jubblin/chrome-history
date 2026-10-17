package web

import (
	"io/fs"
	"regexp"
	"testing"
)

// The hidden-results note must sit inside a persistent role=status live
// region so screen readers announce it when it appears.
func TestMaskNoteIsInLiveRegion(t *testing.T) {
	b, err := fs.ReadFile(FS, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)<div[^>]*id="mask-live"[^>]*>.*?<p[^>]*id="mask-note"[^>]*hidden`)
	if !re.Match(b) {
		t.Fatal("mask-note must be a hidden child of #mask-live")
	}
	if !regexp.MustCompile(`<div[^>]*id="mask-live"[^>]*role="status"`).Match(b) {
		t.Fatal("#mask-live must have role=status")
	}
}
