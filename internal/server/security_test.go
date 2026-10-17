package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"chrome-history/internal/history"
)

func secServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := New([]history.Profile{fixture(t)}, fstest.MapFS{"index.html": {Data: []byte("hi")}})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts
}

// do sends a GET without following redirects.
func do(t *testing.T, ts *httptest.Server, path string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestTokenRequired(t *testing.T) {
	srv, ts := secServer(t)
	tok := srv.Token()
	if len(tok) < 32 || tok == New(nil, nil).Token() {
		t.Fatalf("token must be long and random: %q", tok)
	}
	for _, path := range []string{"/", "/api/profiles", "/api/timeline", "/api/nothing", "/index.html"} {
		for name, hdr := range map[string]map[string]string{
			"missing":       nil,
			"wrong header":  {TokenHeader: tok + "x"},
			"empty header":  {TokenHeader: ""},
			"wrong bearer":  {"Authorization": "Bearer nope"},
			"wrong cookie":  {"Cookie": TokenCookie + "=nope"},
			"basic scheme":  {"Authorization": "Basic " + tok},
			"prefix of tok": {TokenHeader: tok[:len(tok)-1]},
		} {
			resp := do(t, ts, path, hdr)
			if resp.StatusCode != 401 {
				t.Errorf("%s %s: got %d want 401", name, path, resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("%s %s: 401 must be JSON, got %q", name, path, ct)
			}
		}
	}
	for name, hdr := range map[string]map[string]string{
		"header": {TokenHeader: tok},
		"bearer": {"Authorization": "Bearer " + tok},
		"cookie": {"Cookie": TokenCookie + "=" + tok},
	} {
		if resp := do(t, ts, "/api/profiles", hdr); resp.StatusCode != 200 {
			t.Errorf("%s: got %d want 200", name, resp.StatusCode)
		}
	}
	// Host check still applies (403 before token check).
	if resp := do(t, ts, "/api/profiles", map[string]string{TokenHeader: tok, "Host": "evil.example"}); resp.StatusCode != 403 {
		t.Error("bad Host not refused")
	}
}

func TestTokenURLSetsCookieAndRedirects(t *testing.T) {
	srv, ts := secServer(t)
	resp := do(t, ts, "/?token="+srv.Token(), nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("redirect: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	var c *http.Cookie
	for _, x := range resp.Cookies() {
		if x.Name == TokenCookie {
			c = x
		}
	}
	if c == nil || c.Value != srv.Token() {
		t.Fatal("cookie not set")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Errorf("cookie flags: %+v", c)
	}
	if strings.Contains(resp.Header.Get("Location"), srv.Token()) {
		t.Error("token left in redirect URL")
	}
	// Other query parameters survive; token is dropped.
	resp = do(t, ts, "/api/timeline?token="+srv.Token()+"&limit=5", nil)
	if resp.Header.Get("Location") != "/api/timeline?limit=5" {
		t.Errorf("location %q", resp.Header.Get("Location"))
	}
	// Paths that browsers could read as a protocol-relative redirect are refused.
	for _, bad := range []string{`/\evil.example`, "/%5Cevil.example", "/%5cevil.example", "//evil.example/x", "/%2F%2Fevil.example", "/%2f%2fevil.example"} {
		resp = do(t, ts, bad+"?token="+srv.Token(), nil)
		if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
			t.Errorf("%s: status %d location %q, want 400 and no redirect", bad, resp.StatusCode, resp.Header.Get("Location"))
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: content-type %q", bad, ct)
		}
		if resp.Header.Get("Set-Cookie") != "" {
			t.Errorf("%s: cookie set on refused path", bad)
		}
	}
	// A plain nested path redirects to the same path, always starting with one "/".
	resp = do(t, ts, "/api/x%3Fy?token="+srv.Token(), nil)
	if loc := resp.Header.Get("Location"); resp.StatusCode != 302 || loc != "/api/x%3Fy" {
		t.Errorf("plain path: status %d location %q", resp.StatusCode, loc)
	}
	// Wrong URL token: 401, no cookie.
	resp = do(t, ts, "/?token=wrong", nil)
	if resp.StatusCode != 401 || len(resp.Cookies()) != 0 {
		t.Errorf("wrong token: %d cookies=%v", resp.StatusCode, resp.Cookies())
	}
	if resp = do(t, ts, "/?token=", nil); resp.StatusCode != 401 {
		t.Errorf("empty token: %d", resp.StatusCode)
	}
	if resp = do(t, ts, "/?token="+srv.Token()+"&token=x", nil); resp.StatusCode != 401 {
		t.Errorf("duplicate token: %d", resp.StatusCode)
	}
	// Cookie obtained works.
	if resp = do(t, ts, "/api/profiles", map[string]string{"Cookie": TokenCookie + "=" + srv.Token()}); resp.StatusCode != 200 {
		t.Errorf("cookie auth: %d", resp.StatusCode)
	}
}

func TestTokenEqual(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"abc", "abc", true}, {"abc", "abd", false}, {"abc", "ab", false}, {"", "abc", false}, {"abc", "", false}, {"", "", true}} {
		if got := tokenEqual(c.a, c.b); got != c.want {
			t.Errorf("tokenEqual(%q,%q)=%v", c.a, c.b, got)
		}
	}
}

func TestCrossSiteRejected(t *testing.T) {
	srv, ts := secServer(t)
	tok := map[string]string{TokenHeader: srv.Token()}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range tok {
			m[a] = b
		}
		m[k] = v
		return m
	}
	own := "http://" + strings.TrimPrefix(ts.URL, "http://")
	for name, tc := range map[string]struct {
		hdr  map[string]string
		want int
	}{
		"cross-site":          {with("Sec-Fetch-Site", "cross-site"), 403},
		"same-origin":         {with("Sec-Fetch-Site", "same-origin"), 200},
		"none (typed URL)":    {with("Sec-Fetch-Site", "none"), 200},
		"foreign origin":      {with("Origin", "https://evil.example"), 403},
		"null origin":         {with("Origin", "null"), 403},
		"other port origin":   {with("Origin", "http://127.0.0.1:1"), 403},
		"own origin":          {with("Origin", own), 200},
		"no origin":           {tok, 200},
		"foreign + no token":  {map[string]string{"Origin": "https://evil.example"}, 403},
		"cross-site no token": {map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
	} {
		if resp := do(t, ts, "/api/profiles", tc.hdr); resp.StatusCode != tc.want {
			t.Errorf("%s: got %d want %d", name, resp.StatusCode, tc.want)
		}
	}
	if resp := do(t, ts, "/?token="+srv.Token(), map[string]string{"Sec-Fetch-Site": "cross-site"}); resp.StatusCode != 403 {
		t.Errorf("cross-site token URL: %d", resp.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, ts := secServer(t)
	want := map[string]string{
		"Content-Security-Policy":      "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
		"X-Content-Type-Options":       "nosniff",
		"Cache-Control":                "no-store",
	}
	auth := map[string]string{TokenHeader: srv.Token()}
	for name, r := range map[string]*http.Response{
		"index":        do(t, ts, "/", auth),
		"api":          do(t, ts, "/api/profiles", auth),
		"404":          do(t, ts, "/api/nothing", auth),
		"401":          do(t, ts, "/api/profiles", nil),
		"403 host":     do(t, ts, "/", map[string]string{"Host": "evil.example"}),
		"redirect":     do(t, ts, "/?token="+srv.Token(), nil),
		"403 xsite":    do(t, ts, "/", map[string]string{"Origin": "https://evil.example"}),
		"static 404":   do(t, ts, "/missing.js", auth),
		"api bad req":  do(t, ts, "/api/timeline?limit=-1", auth),
		"api no prof.": do(t, ts, "/api/timeline?profile=zz", auth),
	} {
		for h, v := range want {
			if got := r.Header.Get(h); got != v {
				t.Errorf("%s: %s = %q want %q", name, h, got, v)
			}
		}
		if strings.Contains(r.Header.Get("Content-Security-Policy"), "unsafe-") {
			t.Errorf("%s: CSP must not use unsafe-*", name)
		}
	}
}

func TestProfilesOmitEmailAndDir(t *testing.T) {
	p := fixture(t)
	p.Email, p.Name = "someone@example.invalid", "Fixture"
	ts := newTS(New([]history.Profile{p}, nil))
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/api/profiles", nil)
	req.Header.Set(TokenHeader, testToken(ts))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	body := sb.String()
	for _, bad := range []string{"someone@", "example.invalid", `"email"`, `"dir"`, p.Dir, p.Path, p.HistoryPath, os.TempDir()} {
		if bad != "" && strings.Contains(body, bad) {
			t.Errorf("profiles response leaks %q: %s", bad, body)
		}
	}
	if !strings.Contains(body, `"name":"Fixture"`) || !strings.Contains(body, `"id"`) {
		t.Errorf("name/id missing: %s", body)
	}
}

func TestVisitCountCached(t *testing.T) {
	p := fixture(t)
	srv := New([]history.Profile{p}, nil)
	id := ProfileID(p)
	n := srv.visitCount(id, p)
	if n == nil || *n != 5 {
		t.Fatalf("count %v", n)
	}
	// Prove the second call does not touch the database: replace the cached
	// value with a sentinel under the same fingerprint.
	sentinel := 42
	srv.countMu.Lock()
	srv.counts[id] = countEntry{fingerprint(p), &sentinel}
	srv.countMu.Unlock()
	if got := srv.visitCount(id, p); got == nil || *got != 42 {
		t.Fatal("unchanged files must reuse the cached count")
	}
	// Changing size/mtime invalidates it.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p.HistoryPath, later, later); err != nil {
		t.Fatal(err)
	}
	if got := srv.visitCount(id, p); got == nil || *got != 5 {
		t.Fatalf("changed file must be recounted, got %v", got)
	}
}

func TestProfilesDoesNotCopyRepeatedly(t *testing.T) {
	p := fixture(t)
	ts := newTS(New([]history.Profile{p}, nil))
	defer ts.Close()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	for i := 0; i < 3; i++ {
		if code, _ := get(t, ts, "/api/profiles", ""); code != 200 {
			t.Fatal(code)
		}
	}
	// Only the first call may have created a temp copy; none may remain.
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("leftover temp entries: %v", entries)
	}
}
