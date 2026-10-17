package history

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

// ErrSchema is returned (wrapped) when the History DB lacks expected tables/columns.
var ErrSchema = errors.New("history: unsupported or missing schema")

// webkitEpochDelta is microseconds between 1601-01-01 and 1970-01-01 UTC.
const webkitEpochDelta = 11644473600 * 1_000_000

// FromWebKit converts WebKit microseconds (since 1601-01-01 UTC) to time.Time.
// A zero value yields the zero time.Time.
func FromWebKit(us int64) time.Time {
	if us == 0 {
		return time.Time{}
	}
	return time.UnixMicro(us - webkitEpochDelta).UTC()
}

// ToWebKit converts a time to WebKit microseconds; zero time yields 0.
func ToWebKit(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMicro() + webkitEpochDelta
}

var transitionNames = map[int]string{
	0: "link", 1: "typed", 2: "auto_bookmark", 3: "auto_subframe", 4: "manual_subframe",
	5: "generated", 6: "start_page", 7: "form_submit", 8: "reload", 9: "keyword", 10: "keyword_generated",
}

// TransitionName decodes the core transition type (low 8 bits).
func TransitionName(t int64) string {
	if n, ok := transitionNames[int(t&0xFF)]; ok {
		return n
	}
	return "unknown"
}

// Visit is one row of visits joined with urls.
type Visit struct {
	ID         int64         `json:"id"`
	Time       time.Time     `json:"time"`
	URL        string        `json:"url"`
	Title      string        `json:"title"`
	Domain     string        `json:"domain"`
	Transition string        `json:"transition"`
	Duration   time.Duration `json:"duration"` // nanoseconds in JSON
	FromVisit  int64         `json:"fromVisit"`
}

// Filter narrows a query. Zero values mean "no constraint".
type Filter struct {
	From   time.Time `json:"from,omitempty"`   // inclusive
	To     time.Time `json:"to,omitempty"`     // exclusive
	Search string    `json:"search,omitempty"` // case-insensitive substring of url or title
	Domain string    `json:"domain,omitempty"` // host or parent domain
	Limit  int       `json:"limit,omitempty"`
	Offset int       `json:"offset,omitempty"`
	Oldest bool      `json:"oldest,omitempty"` // default is newest first
}

// Reader reads a private copy of a History database.
type Reader struct {
	db     *sql.DB
	tmpDir string
	hasDur bool
}

// OpenProfile opens the profile's History DB via a temp copy.
func OpenProfile(p Profile) (*Reader, error) { return Open(p.HistoryPath) }

// Open copies the History file (plus -wal/-journal) into a temp dir and opens
// the copy. The original is only read, never modified.
func Open(historyPath string) (*Reader, error) {
	tmp, err := os.MkdirTemp("", "chrome-history-*")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Reader, error) { os.RemoveAll(tmp); return nil, err }
	dst := filepath.Join(tmp, "History")
	if err := copyFile(historyPath, dst); err != nil {
		return fail(fmt.Errorf("history: copy database: %w", err))
	}
	for _, suf := range []string{"-wal", "-journal"} {
		if err := copyFile(historyPath+suf, dst+suf); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail(fmt.Errorf("history: copy %s: %w", suf, err))
		}
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dst))
	if err != nil {
		return fail(err)
	}
	db.SetMaxOpenConns(1)
	r := &Reader{db: db, tmpDir: tmp}
	if err := r.checkSchema(); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// Close closes the DB and removes the temp copy.
func (r *Reader) Close() error {
	err := r.db.Close()
	if e := os.RemoveAll(r.tmpDir); err == nil {
		err = e
	}
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (r *Reader) columns(table string) (map[string]bool, error) {
	rows, err := r.db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchema, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		cols[n] = true
	}
	return cols, rows.Err()
}

func (r *Reader) checkSchema() error {
	for table, need := range map[string][]string{
		"urls":   {"id", "url", "title"},
		"visits": {"id", "url", "visit_time", "transition"},
	} {
		cols, err := r.columns(table)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			return fmt.Errorf("%w: table %q not found (not a Chromium History database?)", ErrSchema, table)
		}
		for _, c := range need {
			if !cols[c] {
				return fmt.Errorf("%w: column %s.%s missing", ErrSchema, table, c)
			}
		}
		if table == "visits" {
			r.hasDur = cols["visit_duration"]
		}
	}
	return nil
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (f Filter) where() (string, []any) {
	var conds []string
	var args []any
	if !f.From.IsZero() {
		conds = append(conds, "v.visit_time >= ?")
		args = append(args, ToWebKit(f.From))
	}
	if !f.To.IsZero() {
		conds = append(conds, "v.visit_time < ?")
		args = append(args, ToWebKit(f.To))
	}
	if f.Search != "" {
		p := "%" + likeEscape(f.Search) + "%"
		conds = append(conds, `(u.url LIKE ? ESCAPE '\' OR u.title LIKE ? ESCAPE '\')`)
		args = append(args, p, p)
	}
	if d := strings.ToLower(strings.TrimSpace(f.Domain)); d != "" {
		e := likeEscape(d)
		var ors []string
		for _, pat := range []string{"%://" + e, "%://" + e + "/%", "%://" + e + ":%", "%://%." + e, "%://%." + e + "/%", "%://%." + e + ":%"} {
			ors = append(ors, `LOWER(u.url) LIKE ? ESCAPE '\'`)
			args = append(args, pat)
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

const fromClause = " FROM visits v JOIN urls u ON u.id = v.url"

// Query returns visits matching f, newest first unless f.Oldest.
func (r *Reader) Query(f Filter) ([]Visit, error) {
	var out []Visit
	err := r.each(f, f.Limit, f.Offset, func(v Visit) { out = append(out, v) })
	return out, err
}

// Count returns the number of visits matching f (Limit/Offset ignored).
func (r *Reader) Count(f Filter) (int, error) {
	w, args := f.where()
	var n int
	err := r.db.QueryRow("SELECT COUNT(*)"+fromClause+w, args...).Scan(&n)
	return n, err
}

func (r *Reader) each(f Filter, limit, offset int, fn func(Visit)) error {
	dur := "0"
	if r.hasDur {
		dur = "COALESCE(v.visit_duration,0)"
	}
	from := "0"
	if cols, _ := r.columns("visits"); cols["from_visit"] {
		from = "COALESCE(v.from_visit,0)"
	}
	w, args := f.where()
	order := "DESC"
	if f.Oldest {
		order = "ASC"
	}
	q := "SELECT v.id, v.visit_time, u.url, COALESCE(u.title,''), v.transition, " + dur + ", " + from +
		fromClause + w + " ORDER BY v.visit_time " + order + ", v.id " + order
	if limit > 0 || offset > 0 {
		if limit <= 0 {
			limit = -1
		}
		q += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v Visit
		var vt, tr, du int64
		if err := rows.Scan(&v.ID, &vt, &v.URL, &v.Title, &tr, &du, &v.FromVisit); err != nil {
			return err
		}
		v.Time = FromWebKit(vt)
		v.Transition = TransitionName(tr)
		v.Duration = time.Duration(du) * time.Microsecond
		v.Domain = DomainOf(v.URL)
		fn(v)
	}
	return rows.Err()
}

// DomainOf returns the lower-cased host (without port) of rawURL, or "".
func DomainOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// DayCount is the number of visits on a calendar day.
type DayCount struct {
	Date   string `json:"date"` // YYYY-MM-DD in the requested location
	Visits int    `json:"visits"`
}

// VisitsPerDay aggregates visits per day in loc (nil = UTC), ascending by date.
func (r *Reader) VisitsPerDay(f Filter, loc *time.Location) ([]DayCount, error) {
	if loc == nil {
		loc = time.UTC
	}
	f.Limit, f.Offset = 0, 0
	m := map[string]int{}
	if err := r.each(f, 0, 0, func(v Visit) { m[v.Time.In(loc).Format("2006-01-02")]++ }); err != nil {
		return nil, err
	}
	out := make([]DayCount, 0, len(m))
	for d, n := range m {
		out = append(out, DayCount{d, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// DomainCount is the number of visits to a domain.
type DomainCount struct {
	Domain string `json:"domain"`
	Visits int    `json:"visits"`
}

// TopDomains returns the n most-visited domains (n<=0 means all).
func (r *Reader) TopDomains(f Filter, n int) ([]DomainCount, error) {
	f.Limit, f.Offset = 0, 0
	m := map[string]int{}
	if err := r.each(f, 0, 0, func(v Visit) {
		if v.Domain != "" {
			m[v.Domain]++
		}
	}); err != nil {
		return nil, err
	}
	out := make([]DomainCount, 0, len(m))
	for d, c := range m {
		out = append(out, DomainCount{d, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Visits != out[j].Visits {
			return out[i].Visits > out[j].Visits
		}
		return out[i].Domain < out[j].Domain
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// DefaultSessionGap is the default idle gap that separates sessions.
const DefaultSessionGap = 30 * time.Minute

// Session is a run of visits with no gap larger than the threshold.
type Session struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Visits []Visit   `json:"visits"`
}

// GroupSessions groups visits (any order) into sessions, ascending in time.
// gap<=0 uses DefaultSessionGap.
func GroupSessions(visits []Visit, gap time.Duration) []Session {
	if gap <= 0 {
		gap = DefaultSessionGap
	}
	vs := append([]Visit(nil), visits...)
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].Time.Before(vs[j].Time) })
	var out []Session
	for _, v := range vs {
		if n := len(out); n > 0 && v.Time.Sub(out[n-1].End) <= gap {
			out[n-1].End = v.Time
			out[n-1].Visits = append(out[n-1].Visits, v)
			continue
		}
		out = append(out, Session{Start: v.Time, End: v.Time, Visits: []Visit{v}})
	}
	return out
}

// Sessions queries visits matching f (ignoring Limit/Offset) and groups them.
func (r *Reader) Sessions(f Filter, gap time.Duration) ([]Session, error) {
	f.Limit, f.Offset = 0, 0
	vs, err := r.Query(f)
	if err != nil {
		return nil, err
	}
	return GroupSessions(vs, gap), nil
}
