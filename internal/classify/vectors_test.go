package classify

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The JavaScript port in extension/src/classify is tested against the vectors
// file generated here. TestVectorsFresh fails if the committed file is stale;
// regenerate with: UPDATE_VECTORS=1 go test ./internal/classify -run TestVectorsFresh

const vectorsPath = "../../extension/test/vectors.json"

type vector struct {
	URL  string   `json:"url"`
	Want []string `json:"want"`
}

type userVectors struct {
	Name  string          `json:"name"`
	Rules json.RawMessage `json:"rules"`
	Cases []vector        `json:"cases"`
}

type vectorFile struct {
	Comment string        `json:"comment"`
	Bundled []vector      `json:"bundled"`
	User    []userVectors `json:"user"`
}

func vec(c *Classifier, urls ...string) []vector {
	out := make([]vector, 0, len(urls))
	for _, u := range urls {
		w := c.Classify(u)
		if w == nil {
			w = []string{}
		}
		out = append(out, vector{u, w})
	}
	return out
}

func buildVectors(t *testing.T) []byte {
	t.Helper()
	c := Default()
	var b bundledRules
	if err := json.Unmarshal(bundledJSON, &b); err != nil {
		t.Fatal(err)
	}
	var urls []string
	cats := make([]string, 0, len(b.Categories))
	for cat := range b.Categories {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	for _, cat := range cats {
		r := b.Categories[cat]
		for _, d := range r.Domains {
			urls = append(urls, "https://www."+d+"/", "https://"+d+":8443/a/b?q=1")
		}
		for _, k := range r.Keywords {
			urls = append(urls,
				"https://"+k+".example/",
				"https://shop-"+k+".example/",
				"https://site.example/"+k+"/page",
				"https://site.example/"+k+"s/page",
				"https://site.example/x-"+k+"/page",
				"https://my"+k+".example/",
				"https://site.example/?q="+k,
				"https://SITE.example/"+strings.ToUpper(k),
			)
		}
	}
	allowCats := make([]string, 0, len(b.Allow))
	for cat := range b.Allow {
		allowCats = append(allowCats, cat)
	}
	sort.Strings(allowCats)
	for _, cat := range allowCats {
		for _, d := range b.Allow[cat].Domains {
			urls = append(urls, "https://"+d+"/", "https://sub."+d+"/x")
		}
	}
	urls = append(urls,
		"https://www.bet365.com/en/", "https://BET365.com", "https://sports.bet365.com:8443/x?y=1",
		"https://a.b.c.bbc.co.uk/", "https://bbc.co.uk./", "https://example.co.uk/",
		"https://casino-vpn.example/", "https://porn-casino.example/", "https://casino.example/vpn/porn",
		"https://plain.example/page?q=casino", "https://example.com/", "https://example.org/news",
		"https://notbet365.example/", "https://notbet365.com/", "https://bet365.com.evil.example/",
		"https://bet365-com.example/", "https://mynhs.uk/", "https://example.com/bet365.com",
		"https://example.com/?next=bet365.com", "https://torproject.org.example/", "https://fakepaypal.com/",
		"https://xbbc.co.uk/", "https://127.0.0.1:8080/", "https://[::1]:8080/vpn",
		"http://www.nhs.uk/conditions/", "ftp://files.bbc.co.uk/a", "https://user:pw@www.paypal.com/signin",
		"file:///tmp/casino.html", "chrome://settings/", "about:blank", "", "://bad url",
		"https://games.example/online/Poker/lobby", "https://free.example/xxx/",
		"https://games.example/poker-hand-evaluator", "https://pokerface.example/",
		"https://example.com/Caf%C3%A9/vpn", "mailto:a@casino.example",
	)
	f := vectorFile{
		Comment: "Generated from the Go classifier; do not edit. Regenerate: UPDATE_VECTORS=1 go test ./internal/classify -run TestVectorsFresh",
		Bundled: vec(c, urls...),
	}

	userCases := []struct{ name, rules string }{
		{"add and allow", `{
  "add": {"gambling": {"domains": ["lucky.example", "*.Other.Example"], "keywords": ["lotto"]},
          "finance": {"domains": ["mybank.example"]}},
  "allow": {"news_politics": {"domains": ["bbc.co.uk"]},
            "gambling": {"domains": ["bet365.com"], "keywords": ["pokerface"]},
            "privacy": {"keywords": ["vpn-docs"]}}
}`},
		{"allow domain beats keyword", `{"allow":{"gambling":{"domains":["casino-news.example"]}}}`},
		{"user keyword is a substring", `{"add":{"health":{"keywords":["clinic"]}}}`},
		{"user add not affected by bundled allow", `{"add":{"adult":{"domains":["github.com"]}}}`},
		{"allow keyword needs a match to apply", `{"allow":{"adult":{"keywords":["safe"]}}}`},
	}
	probe := []string{
		"https://lucky.example/", "https://www.lucky.example/", "https://x.other.example/",
		"https://shop.example/lotto/results", "https://shop.example/LOTTOS", "https://mybank.example/",
		"https://notlucky.example/", "https://www.bbc.co.uk/news", "https://bet365.com/",
		"https://sub.bet365.com/", "https://games.example/pokerface/", "https://games.example/poker/",
		"https://lucky.example/pokerface", "https://casino-vpn.example/vpn-docs",
		"https://www.theguardian.com/", "https://casino-news.example/", "https://casino-news.example/x",
		"https://my-clinic.example/", "https://example.com/clinicians", "https://www.nhs.uk/clinic",
		"https://github.com/", "https://sub.github.com/porn", "https://safe.example/porn",
		"https://porn.example/", "https://example.com/",
	}
	for _, uc := range userCases {
		c, err := New(mustParse(t, uc.rules))
		if err != nil {
			t.Fatal(err)
		}
		f.User = append(f.User, userVectors{uc.name, json.RawMessage(uc.rules), vec(c, probe...)})
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(f); err != nil {
		t.Fatal(err)
	}
	// RawMessage keeps the author's formatting; re-indent for a stable file.
	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", " "); err != nil {
		t.Fatal(err)
	}
	return append(bytes.TrimSpace(out.Bytes()), '\n')
}

func mustParse(t *testing.T, js string) *UserRules {
	t.Helper()
	u, err := ParseUserRules(strings.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestVectorsFresh(t *testing.T) {
	want := buildVectors(t)
	path := filepath.FromSlash(vectorsPath)
	if os.Getenv("UPDATE_VECTORS") != "" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (generate with UPDATE_VECTORS=1)", err)
	}
	// Compare ignoring line endings: a Windows checkout may convert LF to CRLF.
	got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatal("extension/test/vectors.json is stale; regenerate with: UPDATE_VECTORS=1 go test ./internal/classify -run TestVectorsFresh")
	}
}
