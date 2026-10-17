package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"chrome-history/internal/history"

	_ "modernc.org/sqlite"
)

var base = time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)

func fixture(t *testing.T) history.Profile {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hp := filepath.Join(dir, "History")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(hp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT)`,
		`CREATE TABLE visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, from_visit INTEGER, transition INTEGER, visit_duration INTEGER)`,
		`INSERT INTO urls VALUES(1,'https://example.com/a','Alpha'),(2,'https://sub.example.com/b','Beta'),(3,'https://other.org/c','Gamma')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	vis := []struct {
		id, url int
		at      time.Duration
	}{{1, 1, 0}, {2, 2, 10 * time.Minute}, {3, 3, 20 * time.Minute}, {4, 1, 2 * time.Hour}, {5, 1, 26 * time.Hour}}
	for _, v := range vis {
		if _, err := db.Exec(`INSERT INTO visits VALUES(?,?,?,?,0,0)`, v.id, v.url, history.ToWebKit(base.Add(v.at)), 0); err != nil {
			t.Fatal(err)
		}
	}
	ps, err := history.ResolvePath(dir)
	if err != nil || len(ps) != 1 {
		t.Fatalf("resolve: %v %v", ps, err)
	}
	return ps[0]
}

func get(t *testing.T, ts *httptest.Server, path string, host string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, m
}

func TestAPI(t *testing.T) {
	p := fixture(t)
	ts := httptest.NewServer(New([]history.Profile{p}, fstest.MapFS{"index.html": {Data: []byte("hi")}}))
	defer ts.Close()

	code, m := get(t, ts, "/api/profiles", "")
	if code != 200 {
		t.Fatal(code, m)
	}
	ps := m["profiles"].([]any)
	id := ps[0].(map[string]any)["id"].(string)
	if len(ps) != 1 || strings.Contains(id, "/") {
		t.Fatalf("profiles %v", ps)
	}
	if vc, ok := ps[0].(map[string]any)["visitCount"].(float64); !ok || vc != 5 {
		t.Fatalf("visitCount %v", ps[0])
	}
	if _, leaked := ps[0].(map[string]any)["path"]; leaked {
		t.Fatal("path leaked")
	}

	code, m = get(t, ts, "/api/timeline?profile="+id, "")
	if code != 200 || m["total"].(float64) != 5 {
		t.Fatal(code, m)
	}
	vs := m["visits"].([]any)
	first := vs[0].(map[string]any)
	if first["time"] != "2024-03-06T12:00:00Z" || first["timeMs"].(float64) != float64(base.Add(26*time.Hour).UnixMilli()) {
		t.Fatalf("newest first / timestamps: %v", first)
	}
	_, m = get(t, ts, "/api/timeline?profile="+id+"&domain=example.com&limit=2&offset=1", "")
	if len(m["visits"].([]any)) != 2 || m["total"].(float64) != 4 {
		t.Fatal(m)
	}
	_, m = get(t, ts, "/api/timeline?profile="+id+"&q=gamma", "")
	if m["total"].(float64) != 1 {
		t.Fatal(m)
	}
	_, m = get(t, ts, "/api/timeline?start=2024-03-05&end=2024-03-05", "")
	if m["total"].(float64) != 4 { // single profile is default; date-only end covers the day
		t.Fatal(m)
	}

	code, m = get(t, ts, "/api/summary?profile="+id, "")
	if code != 200 || m["totalVisits"].(float64) != 5 || len(m["perDay"].([]any)) != 2 ||
		len(m["sessions"].([]any)) != 3 || m["topDomains"].([]any)[0].(map[string]any)["domain"] != "example.com" {
		t.Fatal(code, m)
	}

	code, m = get(t, ts, "/api/domains?profile="+id, "")
	if code != 200 || len(m["domains"].([]any)) != 3 {
		t.Fatal(code, m)
	}

	resp, err := http.Get(ts.URL + "/")
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp)
	}
	resp.Body.Close()
}

func TestErrors(t *testing.T) {
	p := fixture(t)
	ts := httptest.NewServer(New([]history.Profile{p, {HistoryPath: "/nonexistent/x/History", Dir: "x"}}, nil))
	defer ts.Close()
	id := ProfileID(p)

	cases := []struct {
		path, host string
		want       int
	}{
		{"/api/timeline", "", 400},
		{"/api/timeline?profile=nope", "", 404},
		{"/api/timeline?profile=../../etc", "", 404},
		{"/api/timeline?profile=" + id + "&start=garbage", "", 400},
		{"/api/timeline?profile=" + id + "&limit=-1", "", 400},
		{"/api/timeline?profile=" + id + "&start=2024-03-06&end=2024-03-05T00:00:00Z", "", 400},
		{"/api/timeline?profile=" + ProfileID(history.Profile{HistoryPath: "/nonexistent/x/History"}), "", 500},
		{"/api/nothing", "", 404},
		{"/api/profiles", "evil.example.com", 403},
		{"/api/profiles", "localhost:1234", 200},
		{"/api/profiles", "[::1]:1234", 200},
	}
	for _, c := range cases {
		code, m := get(t, ts, c.path, c.host)
		if code != c.want {
			t.Errorf("%s host=%q: got %d want %d", c.path, c.host, code, c.want)
		}
		if code >= 400 && (m == nil || m["error"] == nil) {
			t.Errorf("%s: error not JSON", c.path)
		}
	}
	resp, _ := http.Post(ts.URL+"/api/profiles", "text/plain", nil)
	if resp.StatusCode != 405 {
		t.Errorf("POST: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCategories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hp := filepath.Join(dir, "History")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(hp))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT)`,
		`CREATE TABLE visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, from_visit INTEGER, transition INTEGER, visit_duration INTEGER)`,
		`INSERT INTO urls VALUES(1,'https://www.bet365.com/x','Bets'),(2,'https://www.bbc.co.uk/news','News'),(3,'https://example.com/','Plain'),(4,'https://casino-vpn.example/','Both')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	for i, u := range []int{1, 2, 3, 4, 1} {
		if _, err := db.Exec(`INSERT INTO visits VALUES(?,?,?,0,0,0)`, i+1, u, history.ToWebKit(base.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	ps, err := history.ResolvePath(dir)
	if err != nil || len(ps) != 1 {
		t.Fatal(ps, err)
	}
	ts := httptest.NewServer(New(ps, nil))
	defer ts.Close()

	code, m := get(t, ts, "/api/timeline", "")
	if code != 200 {
		t.Fatal(code, m)
	}
	for _, v := range m["visits"].([]any) {
		vm := v.(map[string]any)
		cats, ok := vm["categories"].([]any)
		if !ok {
			t.Fatalf("categories must be an array: %v", vm)
		}
		if vm["domain"] == "example.com" && len(cats) != 0 {
			t.Fatalf("unflagged visit has categories: %v", vm)
		}
	}
	for q, want := range map[string]float64{
		"category=gambling": 3, "category=gambling,news_politics": 4, "category=news_politics&category=privacy": 2,
		"category=gambling&domain=bet365.com": 2, "category=health": 0,
	} {
		_, m = get(t, ts, "/api/timeline?"+q, "")
		if m["total"].(float64) != want || float64(len(m["visits"].([]any))) != want {
			t.Errorf("%s: total %v want %v", q, m["total"], want)
		}
	}
	if code, _ = get(t, ts, "/api/timeline?category=bogus", ""); code != 400 {
		t.Errorf("bogus category: %d", code)
	}
	if code, _ = get(t, ts, "/api/summary?category=bogus", ""); code != 400 {
		t.Errorf("bogus category summary: %d", code)
	}

	_, m = get(t, ts, "/api/summary", "")
	day := m["perDay"].([]any)[0].(map[string]any)
	fl := day["flagged"].(map[string]any)
	if day["flaggedVisits"].(float64) != 4 || fl["gambling"].(float64) != 3 || fl["news_politics"].(float64) != 1 || fl["privacy"].(float64) != 1 {
		t.Fatalf("per-day flags: %v", day)
	}
	topCats := map[string]int{}
	for _, d := range m["topDomains"].([]any) {
		dm := d.(map[string]any)
		cats, ok := dm["categories"].([]any)
		if !ok {
			t.Fatalf("top domain categories must be an array: %v", dm)
		}
		topCats[dm["domain"].(string)] = len(cats)
	}
	if topCats["www.bet365.com"] != 1 || topCats["example.com"] != 0 || topCats["casino-vpn.example"] != 2 {
		t.Fatalf("top domain categories: %v", topCats)
	}
	_, m = get(t, ts, "/api/summary?category=news_politics", "")
	if m["totalVisits"].(float64) != 1 {
		t.Fatalf("summary filter: %v", m)
	}

	code, m = get(t, ts, "/api/categories", "")
	cs := m["categories"].([]any)
	if code != 200 || len(cs) != 6 || m["flaggedVisits"].(float64) != 4 {
		t.Fatal(code, m)
	}
	got := map[string]string{}
	cnt := map[string]float64{}
	for _, c := range cs {
		cm := c.(map[string]any)
		got[cm["id"].(string)] = cm["label"].(string)
		cnt[cm["id"].(string)] = cm["visits"].(float64)
	}
	if got["news_politics"] != "News & politics" || cnt["gambling"] != 3 || cnt["health"] != 0 {
		t.Fatalf("categories: %v %v", got, cnt)
	}
	_, m = get(t, ts, "/api/categories?start=2024-03-05T11:00:00Z&end=2024-03-05T13:30:00Z", "")
	if m["flaggedVisits"].(float64) != 2 {
		t.Fatalf("range: %v", m)
	}
}
