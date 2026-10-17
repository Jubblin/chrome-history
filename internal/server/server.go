// Package server exposes browser history over a local, read-only JSON API.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chrome-history/internal/classify"
	"chrome-history/internal/history"
)

const (
	defaultLimit = 100
	maxLimit     = 1000
	topDomainsN  = 20
)

// Server serves the API and static UI.
type Server struct {
	profiles map[string]history.Profile
	order    []string
	mux      *http.ServeMux
}

// New builds a Server for the given profiles. static may be nil (API only).
func New(profiles []history.Profile, static fs.FS) *Server {
	s := &Server{profiles: map[string]history.Profile{}, mux: http.NewServeMux()}
	for _, p := range profiles {
		id := ProfileID(p)
		if _, dup := s.profiles[id]; dup {
			continue
		}
		s.profiles[id] = p
		s.order = append(s.order, id)
	}
	s.mux.HandleFunc("/api/profiles", s.handleProfiles)
	s.mux.HandleFunc("/api/timeline", s.handleTimeline)
	s.mux.HandleFunc("/api/summary", s.handleSummary)
	s.mux.HandleFunc("/api/domains", s.handleDomains)
	s.mux.HandleFunc("/api/categories", s.handleCategories)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "unknown endpoint")
	})
	if static != nil {
		s.mux.Handle("/", http.FileServer(http.FS(static)))
	}
	return s
}

// ProfileID returns the opaque token for a profile (hash of its History path).
func ProfileID(p history.Profile) string {
	sum := sha256.Sum256([]byte(p.HistoryPath))
	return hex.EncodeToString(sum[:])[:12]
}

// ServeHTTP implements http.Handler with Host and method checks.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !localHost(r.Host) {
		writeError(w, http.StatusForbidden, "forbidden host")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(w, r)
}

func localHost(hostport string) bool {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	h = strings.Trim(strings.ToLower(h), "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}

type badRequest string

func (b badRequest) Error() string { return string(b) }

// parseTime accepts RFC3339, YYYY-MM-DD (UTC midnight) or epoch milliseconds.
func parseTime(s string) (t time.Time, dateOnly bool, err error) {
	if s == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), false, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true, nil
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(ms).UTC(), false, nil
	}
	return time.Time{}, false, errors.New("invalid time")
}

func intParam(r *http.Request, name string, def, min, max int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return 0, badRequest("invalid " + name)
	}
	if n > max {
		n = max
	}
	return n, nil
}

// rangeFilter parses start/end. A date-only end covers that whole day.
func rangeFilter(r *http.Request) (history.Filter, error) {
	var f history.Filter
	q := r.URL.Query()
	var err error
	if f.From, _, err = parseTime(q.Get("start")); err != nil {
		return f, badRequest("invalid start")
	}
	to, dateOnly, err := parseTime(q.Get("end"))
	if err != nil {
		return f, badRequest("invalid end")
	}
	if dateOnly {
		to = to.AddDate(0, 0, 1)
	}
	f.To = to
	if !f.From.IsZero() && !f.To.IsZero() && !f.To.After(f.From) {
		return f, badRequest("end must be after start")
	}
	return f, nil
}

// categoryParam reads the repeatable / comma-separated `category` parameter.
func categoryParam(r *http.Request) ([]string, error) {
	var names []string
	for _, v := range r.URL.Query()["category"] {
		names = append(names, strings.Split(v, ",")...)
	}
	cats, err := classify.ParseCategories(names)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	return cats, nil
}

// maskParam reads the boolean `maskFlagged` parameter (absent = false).
func maskParam(r *http.Request) (bool, error) {
	v := r.URL.Query().Get("maskFlagged")
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, badRequest("maskFlagged must be true or false")
	}
	return b, nil
}

// withReader resolves the profile, opens a private copy, and runs fn.
func (s *Server) withReader(w http.ResponseWriter, r *http.Request, fn func(*history.Reader) (any, error)) {
	id := r.URL.Query().Get("profile")
	if id == "" && len(s.order) == 1 {
		id = s.order[0]
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing profile")
		return
	}
	p, ok := s.profiles[id]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown profile")
		return
	}
	rd, err := history.OpenProfile(p)
	if err != nil {
		log.Printf("open profile %s: %v", id, err)
		if errors.Is(err, history.ErrSchema) {
			writeError(w, http.StatusUnprocessableEntity, "unsupported history database schema")
		} else {
			writeError(w, http.StatusInternalServerError, "cannot open history database")
		}
		return
	}
	defer rd.Close()
	out, err := fn(rd)
	if err != nil {
		var br badRequest
		if errors.As(err, &br) {
			writeError(w, http.StatusBadRequest, br.Error())
			return
		}
		log.Printf("query profile %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type profileJSON struct {
	ID      string `json:"id"`
	Browser string `json:"browser"`
	Channel string `json:"channel,omitempty"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Dir     string `json:"dir"`
	Kind    string `json:"kind"`
	// VisitCount is omitted when the history database cannot be read.
	VisitCount *int `json:"visitCount,omitempty"`
}

func visitCount(p history.Profile) *int {
	rd, err := history.OpenProfile(p)
	if err != nil {
		return nil
	}
	defer rd.Close()
	n, err := rd.Count(history.Filter{})
	if err != nil {
		return nil
	}
	return &n
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	out := make([]profileJSON, 0, len(s.order))
	for _, id := range s.order {
		p := s.profiles[id]
		out = append(out, profileJSON{id, p.Browser, p.Channel, p.Name, p.Email, p.Dir, p.Kind, visitCount(p)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

type visitJSON struct {
	ID         int64  `json:"id"`
	Time       string `json:"time"`
	TimeMs     int64  `json:"timeMs"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Domain     string `json:"domain"`
	Transition string `json:"transition"`
	DurationMs int64  `json:"durationMs"`
	FromVisit  int64  `json:"fromVisit"`
	// Categories is always an array (empty for unflagged visits).
	Categories []string `json:"categories"`
}

func toVisitJSON(v history.Visit) visitJSON {
	cats := v.Categories
	if cats == nil {
		cats = []string{}
	}
	return visitJSON{v.ID, v.Time.UTC().Format(time.RFC3339Nano), v.Time.UnixMilli(), v.URL, v.Title,
		v.Domain, v.Transition, v.Duration.Milliseconds(), v.FromVisit, cats}
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	f, err := rangeFilter(r)
	if err == nil {
		q := r.URL.Query()
		f.Search, f.Domain = q.Get("q"), q.Get("domain")
		f.Categories, err = categoryParam(r)
	}
	if err == nil {
		f.MaskFlagged, err = maskParam(r)
	}
	if err == nil {
		if f.Limit, err = intParam(r, "limit", defaultLimit, 1, maxLimit); err == nil {
			f.Offset, err = intParam(r, "offset", 0, 0, 1<<30)
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.withReader(w, r, func(rd *history.Reader) (any, error) {
		total, err := rd.Count(f)
		if err != nil {
			return nil, err
		}
		vs, err := rd.Query(f)
		if err != nil {
			return nil, err
		}
		out := make([]visitJSON, 0, len(vs))
		for _, v := range vs {
			out = append(out, toVisitJSON(v))
		}
		return map[string]any{"visits": out, "total": total, "limit": f.Limit, "offset": f.Offset}, nil
	})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	f, err := rangeFilter(r)
	if err == nil {
		f.Categories, err = categoryParam(r)
	}
	if err == nil {
		f.MaskFlagged, err = maskParam(r)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.withReader(w, r, func(rd *history.Reader) (any, error) {
		total, err := rd.Count(f)
		if err != nil {
			return nil, err
		}
		days, err := rd.VisitsPerDay(f, time.UTC)
		if err != nil {
			return nil, err
		}
		doms, err := rd.TopDomains(f, topDomainsN)
		if err != nil {
			return nil, err
		}
		ss, err := rd.Sessions(f, 0)
		if err != nil {
			return nil, err
		}
		type sessionJSON struct {
			Start   string `json:"start"`
			StartMs int64  `json:"startMs"`
			End     string `json:"end"`
			EndMs   int64  `json:"endMs"`
			Visits  int    `json:"visits"`
		}
		sessions := make([]sessionJSON, 0, len(ss))
		for _, x := range ss {
			sessions = append(sessions, sessionJSON{x.Start.Format(time.RFC3339Nano), x.Start.UnixMilli(),
				x.End.Format(time.RFC3339Nano), x.End.UnixMilli(), len(x.Visits)})
		}
		type topDomainJSON struct {
			history.DomainCount
			// Categories classify the domain alone (so the UI can mask it).
			Categories []string `json:"categories"`
		}
		top := make([]topDomainJSON, 0, len(doms))
		for _, d := range doms {
			cats := classify.Classify("https://" + d.Domain + "/")
			if cats == nil {
				cats = []string{}
			}
			top = append(top, topDomainJSON{d, cats})
		}
		return map[string]any{
			"totalVisits": total, "perDay": days, "topDomains": top,
			"sessions": sessions, "sessionGapMinutes": int(history.DefaultSessionGap.Minutes()),
		}, nil
	})
}

func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request) {
	f, err := rangeFilter(r)
	if err == nil {
		f.MaskFlagged, err = maskParam(r)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.withReader(w, r, func(rd *history.Reader) (any, error) {
		doms, err := rd.TopDomains(f, 0)
		if err != nil {
			return nil, err
		}
		return map[string]any{"domains": doms}, nil
	})
}

// categoryLabels are the human-readable names for the category IDs.
var categoryLabels = map[string]string{
	classify.Adult: "Adult", classify.Gambling: "Gambling", classify.Health: "Health",
	classify.Finance: "Finance", classify.Privacy: "Privacy tools", classify.NewsPolitics: "News & politics",
}

// handleCategories lists every category with its visit count in the selected
// date range (the category filter itself is ignored so the chips stay stable).
func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	f, err := rangeFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.withReader(w, r, func(rd *history.Reader) (any, error) {
		days, err := rd.VisitsPerDay(f, time.UTC)
		if err != nil {
			return nil, err
		}
		counts := map[string]int{}
		flagged := 0
		for _, d := range days {
			flagged += d.FlaggedVisits
			for c, n := range d.Flagged {
				counts[c] += n
			}
		}
		type catJSON struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			Visits int    `json:"visits"`
		}
		out := make([]catJSON, 0, len(classify.Categories))
		for _, c := range classify.Categories {
			out = append(out, catJSON{c, categoryLabels[c], counts[c]})
		}
		return map[string]any{"categories": out, "flaggedVisits": flagged}, nil
	})
}
