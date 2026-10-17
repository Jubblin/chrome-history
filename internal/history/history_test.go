package history

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mkDB(t *testing.T, path string, wal bool) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if wal {
		for _, s := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0"} {
			if _, err := db.Exec(s); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, s := range []string{
		`CREATE TABLE urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT, visit_count INTEGER, last_visit_time INTEGER)`,
		`CREATE TABLE visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, from_visit INTEGER, transition INTEGER, visit_duration INTEGER)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

var base = time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)

func seed(t *testing.T, db *sql.DB) {
	t.Helper()
	urls := []struct {
		id       int
		u, title string
	}{
		{1, "https://example.com/a", "Alpha page"},
		{2, "https://sub.example.com:8443/b", "Beta"},
		{3, "https://other.org/c?x=100%", "Gamma_100%"},
	}
	for _, u := range urls {
		db.Exec(`INSERT INTO urls(id,url,title) VALUES(?,?,?)`, u.id, u.u, u.title)
	}
	vis := []struct {
		id, url int
		at      time.Duration
		tr      int64
		dur     int64
	}{
		{1, 1, 0, 1 | 0x20000000, 5_000_000},
		{2, 2, 10 * time.Minute, 0, 0},
		{3, 3, 20 * time.Minute, 8, 1000},
		{4, 1, 2 * time.Hour, 0, 0},
		{5, 1, 26 * time.Hour, 0, 0},
	}
	for _, v := range vis {
		if _, err := db.Exec(`INSERT INTO visits VALUES(?,?,?,?,?,?)`, v.id, v.url, ToWebKit(base.Add(v.at)), v.id-1, v.tr, v.dur); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEpochConversion(t *testing.T) {
	// 13_000_000_000_000_000 us after 1601 -> known value
	if got := FromWebKit(11644473600 * 1_000_000); !got.Equal(time.Unix(0, 0)) {
		t.Fatalf("unix epoch: %v", got)
	}
	if got := ToWebKit(base); !FromWebKit(got).Equal(base) {
		t.Fatalf("roundtrip")
	}
	if !FromWebKit(0).IsZero() {
		t.Fatal("zero")
	}
	if TransitionName(1|0x20000000) != "typed" || TransitionName(0xFF) != "unknown" {
		t.Fatal("transition")
	}
}

func TestQueryAndAggregation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History")
	db := mkDB(t, p, false)
	seed(t, db)
	db.Close()

	r, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	all, err := r.Query(Filter{})
	if err != nil || len(all) != 5 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	if !all[0].Time.After(all[4].Time) {
		t.Fatal("expected newest first")
	}
	old, _ := r.Query(Filter{Oldest: true, Limit: 1})
	if len(old) != 1 || old[0].Transition != "typed" || old[0].Duration != 5*time.Second || !old[0].Time.Equal(base) || old[0].Domain != "example.com" {
		t.Fatalf("oldest=%+v", old)
	}
	if n, _ := r.Count(Filter{Domain: "example.com"}); n != 4 {
		t.Fatalf("domain count %d", n)
	}
	if n, _ := r.Count(Filter{Domain: "sub.example.com"}); n != 1 {
		t.Fatalf("sub count %d", n)
	}
	if n, _ := r.Count(Filter{Domain: "ample.com"}); n != 0 {
		t.Fatalf("suffix must not match: %d", n)
	}
	if n, _ := r.Count(Filter{Search: "gamma_100%"}); n != 1 {
		t.Fatalf("search escape %d", n)
	}
	if n, _ := r.Count(Filter{Search: "gamma_1"}); n != 1 {
		t.Fatalf("underscore literal %d", n)
	}
	if n, _ := r.Count(Filter{Search: "x_y"}); n != 0 {
		t.Fatalf("underscore wildcard leak %d", n)
	}
	if n, _ := r.Count(Filter{From: base.Add(time.Hour), To: base.Add(3 * time.Hour)}); n != 1 {
		t.Fatalf("range %d", n)
	}
	pg, _ := r.Query(Filter{Oldest: true, Limit: 2, Offset: 2})
	if len(pg) != 2 || pg[0].ID != 3 {
		t.Fatalf("paging %+v", pg)
	}
	off, _ := r.Query(Filter{Oldest: true, Offset: 4})
	if len(off) != 1 {
		t.Fatalf("offset only %d", len(off))
	}

	days, _ := r.VisitsPerDay(Filter{}, time.UTC)
	if len(days) != 2 || days[0] != (DayCount{"2024-03-05", 4}) || days[1].Visits != 1 {
		t.Fatalf("days %+v", days)
	}
	top, _ := r.TopDomains(Filter{}, 1)
	if len(top) != 1 || top[0] != (DomainCount{"example.com", 3}) {
		t.Fatalf("top %+v", top)
	}
	ss, _ := r.Sessions(Filter{}, 0)
	if len(ss) != 3 || len(ss[0].Visits) != 3 {
		t.Fatalf("sessions %d", len(ss))
	}
	ss, _ = r.Sessions(Filter{}, 3*time.Hour)
	if len(ss) != 2 {
		t.Fatalf("sessions wide gap %d", len(ss))
	}
}

func TestWALCopy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History")
	db := mkDB(t, p, true)
	defer db.Close()
	seed(t, db)
	if st, err := os.Stat(p + "-wal"); err != nil || st.Size() == 0 {
		t.Fatalf("expected non-empty wal: %v", err)
	}
	before, _ := os.ReadFile(p)
	r, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	n, err := r.Count(Filter{})
	if err != nil || n != 5 {
		t.Fatalf("wal rows not visible: n=%d err=%v", n, err)
	}
	tmp := r.tmpDir
	r.Close()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp not cleaned")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("original modified")
	}
}

func TestSchemaErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History")
	db, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(p))
	db.Exec(`CREATE TABLE other(x)`)
	db.Close()
	if _, err := Open(p); !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema got %v", err)
	}
	// older schema without visit_duration
	p2 := filepath.Join(t.TempDir(), "History")
	db, _ = sql.Open("sqlite", "file:"+filepath.ToSlash(p2))
	db.Exec(`CREATE TABLE urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT)`)
	db.Exec(`CREATE TABLE visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, transition INTEGER)`)
	db.Exec(`INSERT INTO urls VALUES(1,'https://a.com/','t')`)
	db.Exec(`INSERT INTO visits VALUES(1,1,?,0)`, ToWebKit(base))
	db.Close()
	r, err := Open(p2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	vs, err := r.Query(Filter{})
	if err != nil || len(vs) != 1 || vs[0].Duration != 0 {
		t.Fatalf("old schema: %v %+v", err, vs)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error")
	}
}
