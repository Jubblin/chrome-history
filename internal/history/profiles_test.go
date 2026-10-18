package history

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestOperaStyleRootHistory(t *testing.T) {
	// Opera keeps History directly in the user-data dir, not in Default.
	ud := filepath.Join(t.TempDir(), "com.operasoftware.Opera")
	os.MkdirAll(ud, 0o755)
	os.WriteFile(filepath.Join(ud, "History"), []byte("x"), 0o644)
	ps, _ := DiscoverIn([]Root{{"Opera", "stable", ud}})
	if len(ps) != 1 || ps[0].HistoryPath != filepath.Join(ud, "History") || ps[0].Kind != "normal" {
		t.Fatalf("%+v", ps)
	}
}

// fakeHome builds a layout for goos under a temp HOME and returns the
// discovery report. rel is the user-data dir relative to the home.
func fakeHome(t *testing.T, goos string, rel []string, localState string) Report {
	t.Helper()
	home := t.TempDir()
	ud := filepath.Join(append([]string{home}, rel...)...)
	writeProfile(t, ud, "Default")
	writeProfile(t, ud, "Profile 1")
	if localState != "" {
		os.WriteFile(filepath.Join(ud, "Local State"), []byte(localState), 0o644)
	}
	return DiscoverFor(goos, home, filepath.Join(home, "AppData", "Local"), filepath.Join(home, "AppData", "Roaming"))
}

func TestDiscoverForLayouts(t *testing.T) {
	withInfo := `{"profile":{"info_cache":{"Default":{"name":"Work"},"Profile 1":{"name":"Home"}}}}`
	cases := []struct {
		goos string
		rel  []string
	}{
		{"darwin", []string{"Library", "Application Support", "Google", "Chrome"}},
		{"darwin", []string{"Library", "Application Support", "BraveSoftware", "Brave-Browser"}},
		{"darwin", []string{"Library", "Application Support", "Microsoft Edge"}},
		{"linux", []string{".config", "google-chrome"}},
		{"windows", []string{"AppData", "Local", "Google", "Chrome", "User Data"}},
	}
	for _, c := range cases {
		for _, ls := range []string{"", withInfo, "{}", "not json"} {
			rep := fakeHome(t, c.goos, c.rel, ls)
			if len(rep.Profiles) != 2 {
				t.Fatalf("%s %v ls=%q: %d profiles\n%s", c.goos, c.rel, ls, len(rep.Profiles), rep.Explain())
			}
			if ls == withInfo && (rep.Profiles[0].Name != "Work" || rep.Profiles[1].Name != "Home") {
				t.Fatalf("%s: names %+v", c.goos, rep.Profiles)
			}
			if ls != withInfo && rep.Profiles[0].Name != "Default" {
				t.Fatalf("%s: fallback name %q", c.goos, rep.Profiles[0].Name)
			}
		}
	}
}

func TestReportReasons(t *testing.T) {
	home := t.TempDir()
	as := filepath.Join(home, "Library", "Application Support")
	os.MkdirAll(filepath.Join(as, "Chromium", "Crashpad"), 0o755) // exists, no History
	rep := DiscoverFor("darwin", home, "", "")
	if len(rep.Profiles) != 0 {
		t.Fatal("unexpected profiles")
	}
	got := map[string]string{}
	for _, s := range rep.Searched {
		got[s.Path] = s.Reason
	}
	if got[filepath.Join(as, "Chromium")] != ReasonNoHistory || got[filepath.Join(as, "Google", "Chrome")] != ReasonNotFound {
		t.Fatalf("%v", got)
	}
	if rep.PermissionDenied() || strings.Contains(rep.Explain(), "Full Disk Access") {
		t.Fatal("no permission problem expected")
	}
}

func TestReportPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a non-root unix user")
	}
	home := t.TempDir()
	ud := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	writeProfile(t, ud, "Default")
	os.Chmod(filepath.Join(ud, "Default"), 0) // History inside cannot be stat'ed
	t.Cleanup(func() { os.Chmod(filepath.Join(ud, "Default"), 0o755) })
	rep := DiscoverFor("darwin", home, "", "")
	if len(rep.Profiles) != 0 || !rep.PermissionDenied() {
		t.Fatalf("%+v", rep.Searched)
	}
	ex := rep.Explain()
	if !strings.Contains(ex, "Full Disk Access") || !strings.Contains(ex, "--profile PATH") || !strings.Contains(ex, ud) {
		t.Fatal(ex)
	}
	os.Chmod(ud, 0) // whole user-data dir unreadable
	t.Cleanup(func() { os.Chmod(ud, 0o755) })
	rep = DiscoverFor("darwin", home, "", "")
	if !rep.PermissionDenied() {
		t.Fatalf("%+v", rep.Searched)
	}
	if strings.Contains(DiscoverFor("linux", home, "", "").Explain(), "Full Disk Access") {
		t.Fatal("macOS hint on linux")
	}
}

func TestCanaryRootsAndPartialDenial(t *testing.T) {
	has := func(goos string, want string) bool {
		for _, r := range rootsFor(goos, "/h", "/h/L", "/h/R") {
			if r.Browser+" "+r.Channel == want {
				return true
			}
		}
		return false
	}
	for _, c := range []struct{ goos, want string }{
		{"darwin", "Chrome canary"}, {"darwin", "Edge canary"}, {"darwin", "Opera gx"},
		{"windows", "Chrome canary"}, {"windows", "Edge canary"},
	} {
		if !has(c.goos, c.want) {
			t.Errorf("%s: missing %s", c.goos, c.want)
		}
	}
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		return
	}
	home := t.TempDir()
	ud := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	writeProfile(t, ud, "Default")
	writeProfile(t, ud, "Profile 1")
	os.Chmod(filepath.Join(ud, "Profile 1"), 0)
	t.Cleanup(func() { os.Chmod(filepath.Join(ud, "Profile 1"), 0o755) })
	rep := DiscoverFor("darwin", home, "", "")
	if len(rep.Profiles) != 1 || !rep.PermissionDenied() || !strings.Contains(rep.Explain(), "permission denied") {
		t.Fatalf("%+v", rep.Searched)
	}
}
