// Package history discovers Chromium-family browser profiles and reads their
// History SQLite databases safely (always from a temporary copy).
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Profile describes one browser profile that has (or may have) a History DB.
type Profile struct {
	Browser     string `json:"browser"`
	Channel     string `json:"channel,omitempty"`
	UserDataDir string `json:"userDataDir,omitempty"`
	Dir         string `json:"dir"`  // directory name inside UserDataDir, e.g. "Default"
	Path        string `json:"path"` // absolute profile directory
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	HistoryPath string `json:"historyPath"`
	Kind        string `json:"kind"` // "normal", "guest", "system", "custom"
}

// Root is a candidate browser user-data directory.
type Root struct {
	Browser string
	Channel string
	Path    string
}

type localState struct {
	Profile struct {
		InfoCache map[string]struct {
			Name     string `json:"name"`
			UserName string `json:"user_name"`
			GaiaName string `json:"gaia_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

// DefaultRoots returns the well-known user-data dirs for the current OS.
func DefaultRoots() []Root {
	home, _ := os.UserHomeDir()
	return rootsFor(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA"))
}

func rootsFor(goos, home, localAppData, appData string) []Root {
	var r []Root
	add := func(browser, channel, p string) {
		if p != "" {
			r = append(r, Root{browser, channel, p})
		}
	}
	j := filepath.Join
	switch goos {
	case "linux":
		cfg := j(home, ".config")
		for _, c := range []struct{ ch, dir string }{{"stable", "google-chrome"}, {"beta", "google-chrome-beta"}, {"dev", "google-chrome-unstable"}} {
			add("Chrome", c.ch, j(cfg, c.dir))
		}
		add("Chromium", "", j(cfg, "chromium"))
		add("Chromium", "snap", j(home, "snap", "chromium", "common", "chromium"))
		add("Chromium", "flatpak", j(home, ".var", "app", "org.chromium.Chromium", "config", "chromium"))
		add("Chrome", "flatpak", j(home, ".var", "app", "com.google.Chrome", "config", "google-chrome"))
		for _, c := range []struct{ ch, dir string }{{"stable", "microsoft-edge"}, {"beta", "microsoft-edge-beta"}, {"dev", "microsoft-edge-dev"}} {
			add("Edge", c.ch, j(cfg, c.dir))
		}
		add("Edge", "flatpak", j(home, ".var", "app", "com.microsoft.Edge", "config", "microsoft-edge"))
		add("Brave", "stable", j(cfg, "BraveSoftware", "Brave-Browser"))
		add("Brave", "beta", j(cfg, "BraveSoftware", "Brave-Browser-Beta"))
		add("Brave", "nightly", j(cfg, "BraveSoftware", "Brave-Browser-Nightly"))
		add("Brave", "flatpak", j(home, ".var", "app", "com.brave.Browser", "config", "BraveSoftware", "Brave-Browser"))
		add("Brave", "snap", j(home, "snap", "brave", "current", ".config", "BraveSoftware", "Brave-Browser"))
		add("Vivaldi", "stable", j(cfg, "vivaldi"))
		add("Vivaldi", "snapshot", j(cfg, "vivaldi-snapshot"))
		add("Opera", "stable", j(cfg, "opera"))
		add("Opera", "beta", j(cfg, "opera-beta"))
		add("Opera", "developer", j(cfg, "opera-developer"))
	case "darwin":
		as := j(home, "Library", "Application Support")
		add("Chrome", "stable", j(as, "Google", "Chrome"))
		add("Chrome", "beta", j(as, "Google", "Chrome Beta"))
		add("Chrome", "dev", j(as, "Google", "Chrome Dev"))
		add("Chromium", "", j(as, "Chromium"))
		add("Edge", "stable", j(as, "Microsoft Edge"))
		add("Edge", "beta", j(as, "Microsoft Edge Beta"))
		add("Edge", "dev", j(as, "Microsoft Edge Dev"))
		add("Brave", "stable", j(as, "BraveSoftware", "Brave-Browser"))
		add("Brave", "beta", j(as, "BraveSoftware", "Brave-Browser-Beta"))
		add("Brave", "nightly", j(as, "BraveSoftware", "Brave-Browser-Nightly"))
		add("Vivaldi", "stable", j(as, "Vivaldi"))
		add("Opera", "stable", j(as, "com.operasoftware.Opera"))
		add("Opera", "developer", j(as, "com.operasoftware.OperaDeveloper"))
	case "windows":
		if localAppData == "" {
			localAppData = j(home, "AppData", "Local")
		}
		if appData == "" {
			appData = j(home, "AppData", "Roaming")
		}
		add("Chrome", "stable", j(localAppData, "Google", "Chrome", "User Data"))
		add("Chrome", "beta", j(localAppData, "Google", "Chrome Beta", "User Data"))
		add("Chrome", "dev", j(localAppData, "Google", "Chrome Dev", "User Data"))
		add("Chromium", "", j(localAppData, "Chromium", "User Data"))
		add("Edge", "stable", j(localAppData, "Microsoft", "Edge", "User Data"))
		add("Edge", "beta", j(localAppData, "Microsoft", "Edge Beta", "User Data"))
		add("Edge", "dev", j(localAppData, "Microsoft", "Edge Dev", "User Data"))
		add("Brave", "stable", j(localAppData, "BraveSoftware", "Brave-Browser", "User Data"))
		add("Brave", "beta", j(localAppData, "BraveSoftware", "Brave-Browser-Beta", "User Data"))
		add("Vivaldi", "stable", j(localAppData, "Vivaldi", "User Data"))
		add("Opera", "stable", j(appData, "Opera Software", "Opera Stable"))
		add("Opera", "gx", j(appData, "Opera Software", "Opera GX Stable"))
	}
	return r
}

// DiscoverProfiles scans the default roots for the current OS.
func DiscoverProfiles() ([]Profile, error) {
	return DiscoverIn(DefaultRoots())
}

// DiscoverIn scans the given roots; roots that do not exist are skipped.
func DiscoverIn(roots []Root) ([]Profile, error) {
	var out []Profile
	seen := map[string]bool{}
	for _, r := range roots {
		ps, err := scanUserDataDir(r)
		if err != nil {
			continue
		}
		for _, p := range ps {
			if !seen[p.HistoryPath] {
				seen[p.HistoryPath] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// scanUserDataDir reads Local State (if any) and falls back to scanning
// subdirectories for a History file.
func scanUserDataDir(r Root) ([]Profile, error) {
	entries, err := os.ReadDir(r.Path)
	if err != nil {
		return nil, err
	}
	var ls localState
	if b, err := os.ReadFile(filepath.Join(r.Path, "Local State")); err == nil {
		_ = json.Unmarshal(b, &ls)
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		hp := filepath.Join(r.Path, e.Name(), "History")
		if st, err := os.Stat(hp); err != nil || st.IsDir() {
			continue
		}
		p := Profile{
			Browser: r.Browser, Channel: r.Channel, UserDataDir: r.Path,
			Dir: e.Name(), Path: filepath.Join(r.Path, e.Name()), HistoryPath: hp,
			Kind: profileKind(e.Name()),
		}
		if info, ok := ls.Profile.InfoCache[e.Name()]; ok {
			p.Name = info.Name
			p.Email = info.UserName
			if p.Name == "" {
				p.Name = info.GaiaName
			}
		}
		if p.Name == "" {
			p.Name = e.Name()
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

func profileKind(dir string) string {
	switch {
	case dir == "Guest Profile":
		return "guest"
	case dir == "System Profile":
		return "system"
	default:
		return "normal"
	}
}

// ResolvePath accepts a profile dir, a user-data dir, or a copied profile
// (or a direct path to a History file) and returns the profiles found.
func ResolvePath(path string) ([]Profile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("history: path %q: %w", path, err)
	}
	if !st.IsDir() {
		if filepath.Base(abs) != "History" {
			return nil, fmt.Errorf("history: %q is not a directory or History file", path)
		}
		abs = filepath.Dir(abs)
	}
	if hp := filepath.Join(abs, "History"); fileExists(hp) {
		p := Profile{
			Browser: "Unknown", Dir: filepath.Base(abs), Path: abs, HistoryPath: hp,
			Name: filepath.Base(abs), Kind: "custom",
		}
		parent := filepath.Dir(abs)
		if ps, err := scanUserDataDir(Root{Browser: "Unknown", Path: parent}); err == nil {
			for _, q := range ps {
				if q.Dir == p.Dir {
					p = q
					break
				}
			}
		}
		return []Profile{p}, nil
	}
	ps, err := scanUserDataDir(Root{Browser: "Unknown", Path: abs})
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, errors.New("history: no History file found under " + abs)
	}
	return ps, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Label is a short human description that avoids URLs.
func (p Profile) Label() string {
	s := p.Browser
	if p.Channel != "" {
		s += " " + p.Channel
	}
	s += " / " + p.Name
	if p.Email != "" {
		s += " <" + strings.TrimSpace(p.Email) + ">"
	}
	return s
}
