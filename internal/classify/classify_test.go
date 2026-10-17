package classify

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBundledRulesValid(t *testing.T) {
	if _, err := newClassifier(nil); err != nil {
		t.Fatal(err)
	}
	for _, cat := range Categories {
		if bit(cat) == 0 {
			t.Errorf("category %s has no bit", cat)
		}
	}
	// every category has at least one bundled domain
	c := Default()
	for _, cat := range Categories {
		n := 0
		for _, m := range c.bundledDomains {
			if m&bit(cat) != 0 {
				n++
			}
		}
		if n == 0 {
			t.Errorf("no bundled domains for %s", cat)
		}
	}
}

func TestBundledMatches(t *testing.T) {
	tests := []struct {
		url  string
		want []string
	}{
		{"https://www.bet365.com/en/", []string{Gambling}},
		{"https://BET365.com", []string{Gambling}},
		{"https://sports.bet365.com:8443/x?y=1", []string{Gambling}},
		{"https://pornhub.com/", []string{Adult}},
		{"https://www.nhs.uk/conditions/", []string{Health}},
		{"https://www.paypal.com/signin", []string{Finance}},
		{"https://www.torproject.org/download", []string{Privacy}},
		{"https://nordvpn.com/", []string{Privacy}},
		{"https://www.bbc.co.uk/news", []string{NewsPolitics}},
		{"https://labour.org.uk/", []string{NewsPolitics}},
		{"https://www.greenpeace.org/", []string{NewsPolitics}},
		// keyword on host and path
		{"https://fun-casino-games.example/", []string{Gambling}},
		{"https://games.example/online/Poker/lobby", []string{Gambling}},
		{"https://vpn.corp.example/login", []string{Privacy}},
		{"https://free.example/xxx/", []string{Adult}},
		// multiple categories: sorted
		{"https://casino-vpn.example/", []string{Gambling, Privacy}},
		{"https://porn-casino.example/", []string{Adult, Gambling}},
		{"https://casino.example/vpn/porn", []string{Adult, Gambling, Privacy}},
		// keyword in query only does not count
		{"https://plain.example/page?q=casino", nil},
		// unflagged / non-web
		{"https://example.com/", nil},
		{"https://example.org/news", nil},
		{"file:///tmp/casino.html", nil},
		{"chrome://settings/", nil},
		{"about:blank", nil},
		{"", nil},
		{"://bad url", nil},
		{"https://127.0.0.1:8080/", nil},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := Classify(tt.url); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestLookalikesDoNotMatch(t *testing.T) {
	for _, u := range []string{
		"https://notbet365.example/",
		"https://notbet365.com/",
		"https://bet365.com.evil.example/",
		"https://bet365-com.example/",
		"https://mynhs.uk/",
		"https://example.com/bet365.com",
		"https://example.com/?next=bet365.com",
		"https://torproject.org.example/",
		"https://fakepaypal.com/",
		"https://xbbc.co.uk/",
	} {
		if got := Classify(u); got != nil {
			t.Errorf("%s: unexpected %v", u, got)
		}
	}
}

func TestSubdomainsAndTrailingDot(t *testing.T) {
	if got := Classify("https://a.b.c.bbc.co.uk/"); !reflect.DeepEqual(got, []string{NewsPolitics}) {
		t.Errorf("deep subdomain: %v", got)
	}
	if got := Classify("https://bbc.co.uk./"); !reflect.DeepEqual(got, []string{NewsPolitics}) {
		t.Errorf("trailing dot: %v", got)
	}
	// a suffix like co.uk alone is not a rule
	if got := Classify("https://example.co.uk/"); got != nil {
		t.Errorf("co.uk: %v", got)
	}
}

func userClassifier(t *testing.T, js string) *Classifier {
	t.Helper()
	u, err := ParseUserRules(strings.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(u)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUserAddAndAllow(t *testing.T) {
	c := userClassifier(t, `{
	  "add":   {"gambling": {"domains": ["lucky.example", "*.Other.Example"], "keywords": ["lotto"]},
	            "finance":  {"domains": ["mybank.example"]}},
	  "allow": {"news_politics": {"domains": ["bbc.co.uk"]},
	            "gambling": {"domains": ["bet365.com"], "keywords": ["pokerface"]},
	            "privacy": {"keywords": ["vpn-docs"]}}
	}`)
	tests := []struct {
		url  string
		want []string
	}{
		{"https://lucky.example/", []string{Gambling}},
		{"https://www.lucky.example/", []string{Gambling}},
		{"https://x.other.example/", []string{Gambling}},
		{"https://shop.example/lotto/results", []string{Gambling}},
		{"https://mybank.example/", []string{Finance}},
		{"https://notlucky.example/", nil},
		// allow beats bundled
		{"https://www.bbc.co.uk/news", nil},
		{"https://bet365.com/", nil},
		{"https://sub.bet365.com/", nil},
		{"https://games.example/pokerface/", nil},
		{"https://games.example/poker/", []string{Gambling}},
		// allow beats user add
		{"https://lucky.example/pokerface", nil},
		// allow is per category: other categories remain
		{"https://casino-vpn.example/vpn-docs", []string{Gambling}},
		// bundled rules for untouched categories remain
		{"https://www.theguardian.com/", []string{NewsPolitics}},
	}
	for _, tt := range tests {
		if got := c.Classify(tt.url); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v want %v", tt.url, got, tt.want)
		}
	}
	// user rules do not leak into the default classifier
	if got := Classify("https://lucky.example/"); got != nil {
		t.Errorf("default classifier polluted: %v", got)
	}
}

func TestAllowDomainBeatsKeyword(t *testing.T) {
	c := userClassifier(t, `{"allow":{"gambling":{"domains":["casino-news.example"]}}}`)
	if got := c.Classify("https://casino-news.example/"); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestMalformedRules(t *testing.T) {
	bad := map[string]string{
		"not json":          `{`,
		"empty":             ``,
		"array":             `[]`,
		"unknown field":     `{"adds":{}}`,
		"unknown category":  `{"add":{"weather":{"domains":["x.example"]}}}`,
		"unknown cat allow": `{"allow":{"Gambling":{"domains":["x.example"]}}}`,
		"bad domain url":    `{"add":{"adult":{"domains":["https://x.example/"]}}}`,
		"bad domain space":  `{"add":{"adult":{"domains":["x y.example"]}}}`,
		"no dot":            `{"add":{"adult":{"domains":["localhostish"]}}}`,
		"empty domain":      `{"add":{"adult":{"domains":[""]}}}`,
		"short keyword":     `{"add":{"adult":{"keywords":["ab"]}}}`,
		"empty keyword":     `{"add":{"adult":{"keywords":[" "]}}}`,
		"slash keyword":     `{"add":{"adult":{"keywords":["a/b/c"]}}}`,
		"wrong type":        `{"add":{"adult":{"domains":"x.example"}}}`,
		"unknown cat key":   `{"add":{"adult":{"hosts":["x.example"]}}}`,
		"trailing data":     `{} {}`,
	}
	for name, js := range bad {
		if _, err := ParseUserRules(strings.NewReader(js)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	for _, js := range []string{`{}`, `{"add":{}}`, `{"allow":{"adult":{}}}`} {
		if _, err := ParseUserRules(strings.NewReader(js)); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	good := write("good.json", `{"add":{"adult":{"domains":["x.example"]}}}`)
	c, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Classify("https://x.example/"); !reflect.DeepEqual(got, []string{Adult}) {
		t.Errorf("got %v", got)
	}

	bad := write("bad.json", `{"add":{"nope":{}}}`)
	_, err = Load(bad)
	if err == nil || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("want clear error naming file and problem, got %v", err)
	}

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("explicit missing file must error")
	}

	// default path: missing is fine, present is used, invalid errors
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	t.Setenv("AppData", cfg)
	c, err = Load("")
	if err != nil || c != Default() {
		t.Fatalf("missing default file should give bundled rules: %v", err)
	}
	p, _ := DefaultRulesPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"add":{"adult":{"domains":["y.example"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Classify("https://y.example/"); !reflect.DeepEqual(got, []string{Adult}) {
		t.Errorf("default file not used: %v", got)
	}
	if err := os.WriteFile(p, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(""); err == nil {
		t.Error("invalid default file must error")
	}
}

func TestParseCategoriesAndHasAny(t *testing.T) {
	got, err := ParseCategories([]string{"Gambling", " adult", "adult", ""})
	if err != nil || !reflect.DeepEqual(got, []string{Adult, Gambling}) {
		t.Errorf("%v %v", got, err)
	}
	if _, err := ParseCategories([]string{"bogus"}); err == nil {
		t.Error("expected error")
	}
	if !HasAny(nil, nil) || HasAny(nil, []string{Adult}) || !HasAny([]string{Adult, Health}, []string{Health}) {
		t.Error("HasAny")
	}
}

func TestPerformance100k(t *testing.T) {
	c := userClassifier(t, `{"add":{"adult":{"keywords":["lotto"]}},"allow":{"gambling":{"keywords":["pokerface"]}}}`)
	urls := make([]string, 100_000)
	hits := 0
	for i := range urls {
		switch i % 5 {
		case 0:
			urls[i] = fmt.Sprintf("https://site%d.example/path/%d/page?q=%d", i, i, i)
		case 1:
			urls[i] = fmt.Sprintf("https://sub%d.bet365.com/x/%d", i, i)
			hits++
		case 2:
			urls[i] = fmt.Sprintf("https://www.bbc.co.uk/news/%d", i)
			hits++
		case 3:
			urls[i] = fmt.Sprintf("https://a.b.c.d.e.deep%d.example/%d/long/path/segments/here", i, i)
		default:
			urls[i] = fmt.Sprintf("https://x%d.example/casino/%d", i, i)
			hits++
		}
	}
	start := time.Now()
	got := 0
	for _, u := range urls {
		if len(c.Classify(u)) > 0 {
			got++
		}
	}
	d := time.Since(start)
	if got != hits {
		t.Errorf("hits = %d want %d", got, hits)
	}
	t.Logf("100k URLs in %v", d)
	if d > 5*time.Second {
		t.Errorf("too slow: %v", d)
	}
}

func TestConcurrentUse(t *testing.T) {
	c := Default()
	done := make(chan bool)
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				c.Classify("https://www.bet365.com/")
			}
			done <- true
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
