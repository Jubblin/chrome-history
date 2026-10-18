package history

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProfile(t *testing.T, ud, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(ud, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ud, dir, "History"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverWithLocalState(t *testing.T) {
	ud := filepath.Join(t.TempDir(), "google-chrome")
	for _, d := range []string{"Default", "Profile 1", "Guest Profile", "System Profile"} {
		writeProfile(t, ud, d)
	}
	os.MkdirAll(filepath.Join(ud, "Crashpad"), 0o755)
	ls := `{"profile":{"info_cache":{"Default":{"name":"Person 1","user_name":"a@example.com"},"Profile 1":{"name":"","gaia_name":"Gaia"}}}}`
	os.WriteFile(filepath.Join(ud, "Local State"), []byte(ls), 0o644)

	ps, err := DiscoverIn([]Root{{"Chrome", "stable", ud}, {"Brave", "", filepath.Join(ud, "nope")}})
	if err != nil || len(ps) != 4 {
		t.Fatalf("%d %v", len(ps), err)
	}
	by := map[string]Profile{}
	for _, p := range ps {
		by[p.Dir] = p
	}
	if by["Default"].Name != "Person 1" || by["Default"].Email != "a@example.com" || by["Default"].Kind != "normal" {
		t.Fatalf("%+v", by["Default"])
	}
	if by["Profile 1"].Name != "Gaia" || by["Guest Profile"].Kind != "guest" || by["System Profile"].Kind != "system" {
		t.Fatal("kinds/names")
	}
}

func TestDiscoverNoLocalState(t *testing.T) {
	ud := t.TempDir()
	writeProfile(t, ud, "Default")
	ps, _ := DiscoverIn([]Root{{"Chromium", "", ud}})
	if len(ps) != 1 || ps[0].Name != "Default" {
		t.Fatalf("%+v", ps)
	}
}

func TestResolvePath(t *testing.T) {
	ud := t.TempDir()
	writeProfile(t, ud, "Default")
	writeProfile(t, ud, "Profile 2")
	if ps, err := ResolvePath(ud); err != nil || len(ps) != 2 {
		t.Fatalf("userdata: %v %d", err, len(ps))
	}
	if ps, err := ResolvePath(filepath.Join(ud, "Profile 2")); err != nil || len(ps) != 1 || ps[0].Dir != "Profile 2" {
		t.Fatalf("profile dir: %v %+v", err, ps)
	}
	if ps, err := ResolvePath(filepath.Join(ud, "Default", "History")); err != nil || len(ps) != 1 {
		t.Fatalf("file: %v", err)
	}
	if _, err := ResolvePath(t.TempDir()); err == nil {
		t.Fatal("expected error for empty dir")
	}
	if _, err := ResolvePath(filepath.Join(ud, "missing")); err == nil {
		t.Fatal("expected error")
	}
}

func TestRootsFor(t *testing.T) {
	for _, os := range []string{"linux", "darwin", "windows"} {
		rs := rootsFor(os, "/h", "/h/L", "/h/R")
		if len(rs) < 8 {
			t.Fatalf("%s: %d roots", os, len(rs))
		}
	}
}
