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

// The CSP forbids inline script and style, so the UI must not rely on them.
func TestNoInlineScriptOrStyle(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	bad := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<style\b`),
		regexp.MustCompile(`(?i)\sstyle\s*=`),
		regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
		regexp.MustCompile(`(?i)javascript:`),
		regexp.MustCompile(`\.style\.cssText|setAttribute\(\s*['"]style['"]|\.innerHTML|\.insertAdjacentHTML|\beval\(|new Function\(`),
	}
	for _, e := range entries {
		if !regexp.MustCompile(`\.(html|js)$`).MatchString(e.Name()) {
			continue
		}
		b, _ := fs.ReadFile(FS, e.Name())
		for _, m := range regexp.MustCompile(`(?is)<script[^>]*>`).FindAll(b, -1) {
			if !regexp.MustCompile(`(?i)\ssrc=`).Match(m) {
				t.Errorf("%s: inline script %q", e.Name(), m)
			}
		}
		for _, re := range bad {
			if m := re.Find(b); m != nil {
				t.Errorf("%s: CSP-incompatible construct %q", e.Name(), m)
			}
		}
	}
}
