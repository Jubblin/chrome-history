package history

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"history-timeline/internal/classify"
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
	if len(days) != 2 || days[0].Date != "2024-03-05" || days[0].Visits != 4 || days[1].Visits != 1 {
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

func TestCategoriesFilterAndFlagCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "History")
	db := mkDB(t, path, false)
	urls := []string{
		"https://example.com/a",       // 1 none
		"https://www.bet365.com/x",    // 2 gambling
		"https://casino-vpn.example/", // 3 gambling+privacy
		"https://www.bbc.co.uk/news",  // 4 news_politics
		"https://example.org/b",       // 5 none
	}
	for i, u := range urls {
		db.Exec(`INSERT INTO urls(id,url,title) VALUES(?,?,?)`, i+1, u, "t")
		day := time.Duration(0)
		if i >= 3 {
			day = 24 * time.Hour
		}
		db.Exec(`INSERT INTO visits(id,url,visit_time,from_visit,transition,visit_duration) VALUES(?,?,?,0,0,0)`,
			i+1, i+1, ToWebKit(base.Add(day+time.Duration(i)*time.Minute)))
	}
	db.Close()

	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	all, _ := r.Query(Filter{Oldest: true})
	wantCats := [][]string{nil, {"gambling"}, {"gambling", "privacy"}, {"news_politics"}, nil}
	for i, v := range all {
		if len(v.Categories) != len(wantCats[i]) {
			t.Errorf("visit %d: %v want %v", i, v.Categories, wantCats[i])
		}
	}

	f := Filter{Categories: []string{"gambling"}, Oldest: true}
	got, _ := r.Query(f)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Errorf("gambling filter: %+v", got)
	}
	if n, _ := r.Count(f); n != 2 {
		t.Errorf("count = %d", n)
	}
	f.Limit, f.Offset = 1, 1
	if got, _ := r.Query(f); len(got) != 1 || got[0].ID != 3 {
		t.Errorf("paged: %+v", got)
	}
	f.Offset = 5
	if got, _ := r.Query(f); len(got) != 0 {
		t.Errorf("offset past end: %+v", got)
	}
	// multiple categories are OR'd
	if n, _ := r.Count(Filter{Categories: []string{"privacy", "news_politics"}}); n != 2 {
		t.Errorf("or count = %d", n)
	}
	// combines with SQL filters
	if n, _ := r.Count(Filter{Categories: []string{"gambling"}, Search: "casino"}); n != 1 {
		t.Errorf("combined count = %d", n)
	}

	days, err := r.VisitsPerDay(Filter{}, nil)
	if err != nil || len(days) != 2 {
		t.Fatalf("%v %v", days, err)
	}
	d1, d2 := days[0], days[1]
	if d1.Visits != 3 || d1.FlaggedVisits != 2 || d1.Flagged["gambling"] != 2 || d1.Flagged["privacy"] != 1 {
		t.Errorf("day1: %+v", d1)
	}
	if d2.Visits != 2 || d2.FlaggedVisits != 1 || d2.Flagged["news_politics"] != 1 || len(d2.Flagged) != 1 {
		t.Errorf("day2: %+v", d2)
	}

	// per-reader classifier override with an allow rule
	c, _ := classify.New(&classify.UserRules{Allow: map[string]classify.CategoryRules{"gambling": {Domains: []string{"bet365.com"}}}})
	r.SetClassifier(c)
	if n, _ := r.Count(Filter{Categories: []string{"gambling"}}); n != 1 {
		t.Errorf("after allow: %d", n)
	}
}

func TestMaskFlagged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "History")
	db := mkDB(t, path, false)
	rows := [][2]string{
		{"https://example.com/a", "Plain page"},
		{"https://www.bet365.com/x", "Secret bets"},
		{"https://example.org/b", "Other"},
	}
	for i, u := range rows {
		db.Exec(`INSERT INTO urls(id,url,title) VALUES(?,?,?)`, i+1, u[0], u[1])
		db.Exec(`INSERT INTO visits(id,url,visit_time,from_visit,transition,visit_duration) VALUES(?,?,?,0,0,0)`,
			i+1, i+1, ToWebKit(base.Add(time.Duration(i)*time.Minute)))
	}
	db.Close()
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	count := func(f Filter) int {
		n, err := r.Count(f)
		if err != nil {
			t.Fatal(err)
		}
		vs, err := r.Query(f)
		if err != nil || len(vs) != n {
			t.Fatalf("query %d vs count %d (%v)", len(vs), n, err)
		}
		return n
	}
	for _, f := range []Filter{{Search: "bets"}, {Search: "bet365"}, {Domain: "bet365.com"}, {Domain: "www.bet365.com"}} {
		if n := count(f); n != 1 {
			t.Errorf("unmasked %+v = %d, want 1", f, n)
		}
		f.MaskFlagged = true
		if n := count(f); n != 0 {
			t.Errorf("masked %+v = %d, want 0", f, n)
		}
	}
	if n := count(Filter{Search: "plain", MaskFlagged: true}); n != 1 {
		t.Errorf("unflagged search masked = %d", n)
	}
	if n := count(Filter{Domain: "example.com", MaskFlagged: true}); n != 1 {
		t.Errorf("unflagged domain masked = %d", n)
	}
	// without text/domain filters, flagged visits are still returned
	if n := count(Filter{MaskFlagged: true}); n != 3 {
		t.Errorf("masked no filter = %d", n)
	}
	if n := count(Filter{MaskFlagged: true, Categories: []string{"gambling"}}); n != 1 {
		t.Errorf("masked category = %d", n)
	}
	if n := count(Filter{MaskFlagged: true, Categories: []string{"gambling"}, Search: "bets"}); n != 0 {
		t.Errorf("masked category+search = %d", n)
	}
	// paging stays consistent with the filter
	vs, _ := r.Query(Filter{Search: "e", MaskFlagged: true, Limit: 1, Offset: 1, Oldest: true})
	if len(vs) != 1 || vs[0].ID != 3 {
		t.Errorf("paged: %+v", vs)
	}
	ds, _ := r.TopDomains(Filter{MaskFlagged: true}, 0)
	if len(ds) != 2 {
		t.Errorf("masked top domains: %+v", ds)
	}
	for _, d := range ds {
		if d.Domain == "www.bet365.com" {
			t.Errorf("flagged domain leaked: %+v", ds)
		}
	}
	if ds, _ := r.TopDomains(Filter{}, 0); len(ds) != 3 {
		t.Errorf("unmasked top domains: %+v", ds)
	}
}
