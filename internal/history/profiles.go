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

// Skip reasons reported for each searched directory.
const (
	ReasonFound      = "found"
	ReasonNotFound   = "not found"
	ReasonNoHistory  = "no History file"
	ReasonPermission = "permission denied"
	ReasonUnreadable = "unreadable"
)

// RootResult records what happened when one candidate directory was searched.
type RootResult struct {
	Root
	Reason   string // one of the Reason* constants
	Detail   string // extra context, e.g. which entry was denied; never URLs or titles
	Profiles int
}

// Report is the outcome of a discovery run: the profiles found and every
// directory that was searched.
type Report struct {
	GOOS     string
	Profiles []Profile
	Searched []RootResult
}

// PermissionDenied reports whether any searched directory was refused.
func (r Report) PermissionDenied() bool {
	for _, s := range r.Searched {
		if s.Reason == ReasonPermission {
			return true
		}
	}
	return false
}

// Explain describes which directories were searched and why none (or each)
// was used. It contains paths and reasons only.
func (r Report) Explain() string {
	var b strings.Builder
	b.WriteString("searched:\n")
	for _, s := range r.Searched {
		label := s.Browser
		if s.Channel != "" {
			label += " " + s.Channel
		}
		fmt.Fprintf(&b, "  %s: %s: %s", label, s.Path, s.Reason)
		if s.Detail != "" {
			fmt.Fprintf(&b, " (%s)", s.Detail)
		}
		b.WriteByte('\n')
	}
	if r.PermissionDenied() {
		b.WriteString(permissionHelp(r.GOOS))
	}
	return b.String()
}

func permissionHelp(goos string) string {
	if goos == "darwin" {
		return "\nmacOS blocked access to a browser folder. Give your terminal app (Terminal, iTerm, ...) access in\n" +
			"System Settings > Privacy & Security > Full Disk Access, then quit and reopen the terminal and run this again.\n" +
			"Alternatively copy the profile folder somewhere readable and pass it with --profile PATH.\n"
	}
	return "\nA browser folder could not be read because of its permissions. Fix the permissions, or copy the profile\nand pass it with --profile PATH.\n"
}

// DiscoverProfiles scans the default roots for the current OS.
func DiscoverProfiles() ([]Profile, error) {
	return DiscoverIn(DefaultRoots())
}

// DiscoverReport scans the default roots for the current OS and reports on
// every directory searched.
func DiscoverReport() Report {
	home, _ := os.UserHomeDir()
	return DiscoverFor(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA"))
}

// DiscoverFor is DiscoverReport with the OS name and directories injected.
func DiscoverFor(goos, home, localAppData, appData string) Report {
	rep := ReportIn(rootsFor(goos, home, localAppData, appData))
	rep.GOOS = goos
	return rep
}

// DiscoverIn scans the given roots; roots that do not exist are skipped.
func DiscoverIn(roots []Root) ([]Profile, error) {
	return ReportIn(roots).Profiles, nil
}

// ReportIn scans the given roots and records the outcome for each.
func ReportIn(roots []Root) Report {
	var rep Report
	seen := map[string]bool{}
	for _, r := range roots {
		ps, res := scanUserDataDir(r)
		n := 0
		for _, p := range ps {
			if !seen[p.HistoryPath] {
				seen[p.HistoryPath] = true
				rep.Profiles = append(rep.Profiles, p)
				n++
			}
		}
		res.Profiles = n
		rep.Searched = append(rep.Searched, res)
	}
	return rep
}

func classifyErr(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ReasonNotFound
	case errors.Is(err, os.ErrPermission):
		return ReasonPermission
	default:
		return ReasonUnreadable
	}
}

// scanUserDataDir reads Local State (if any) and scans the directory and its
// subdirectories for History files. Opera keeps History directly in the
// user-data directory, so that is checked too.
func scanUserDataDir(r Root) ([]Profile, RootResult) {
	res := RootResult{Root: r}
	entries, err := os.ReadDir(r.Path)
	if err != nil {
		res.Reason = classifyErr(err)
		if res.Reason == ReasonUnreadable {
			res.Detail = err.Error()
		}
		return nil, res
	}
	var ls localState
	if b, err := os.ReadFile(filepath.Join(r.Path, "Local State")); err == nil {
		_ = json.Unmarshal(b, &ls)
	}
	var out []Profile
	denied, other := 0, 0
	add := func(dir, profPath string) {
		hp := filepath.Join(profPath, "History")
		st, err := os.Stat(hp)
		if err != nil {
			switch classifyErr(err) {
			case ReasonPermission:
				denied++
			case ReasonUnreadable:
				other++
			}
			return
		}
		if st.IsDir() {
			return
		}
		p := Profile{
			Browser: r.Browser, Channel: r.Channel, UserDataDir: r.Path,
			Dir: dir, Path: profPath, HistoryPath: hp,
			Kind: profileKind(dir),
		}
		if info, ok := ls.Profile.InfoCache[dir]; ok {
			p.Name = info.Name
			p.Email = info.UserName
			if p.Name == "" {
				p.Name = info.GaiaName
			}
		}
		if p.Name == "" {
			p.Name = dir
		}
		out = append(out, p)
	}
	add(filepath.Base(r.Path), r.Path)
	if len(out) > 0 {
		out[0].Kind = "normal"
	}
	for _, e := range entries {
		// Follow symlinked profile dirs too (e.Type() would not).
		if st, err := os.Stat(filepath.Join(r.Path, e.Name())); err != nil || !st.IsDir() {
			continue
		}
		add(e.Name(), filepath.Join(r.Path, e.Name()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	switch {
	case len(out) > 0:
		res.Reason = ReasonFound
	case denied > 0:
		res.Reason = ReasonPermission
		res.Detail = fmt.Sprintf("%d profile folder(s) could not be read", denied)
	case other > 0:
		res.Reason = ReasonUnreadable
		res.Detail = fmt.Sprintf("%d profile folder(s) could not be read", other)
	default:
		res.Reason = ReasonNoHistory
	}
	return out, res
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
		ps, _ := scanUserDataDir(Root{Browser: "Unknown", Path: parent})
		for _, q := range ps {
			if q.Dir == p.Dir {
				p = q
				break
			}
		}
		return []Profile{p}, nil
	}
	ps, res := scanUserDataDir(Root{Browser: "Unknown", Path: abs})
	if len(ps) == 0 {
		if res.Reason == ReasonPermission {
			return nil, errors.New("history: permission denied reading " + abs + "\n" + permissionHelp(runtime.GOOS))
		}
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
