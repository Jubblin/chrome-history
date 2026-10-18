package classify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// intentionalOverlaps lists domains that may appear in more than one category.
var intentionalOverlaps = map[string]bool{}

func loadBundled(t *testing.T) bundledRules {
	t.Helper()
	raw, err := os.ReadFile("rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var b bundledRules
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		t.Fatal(err)
	}
	if b.Version != 1 {
		t.Errorf("version = %d", b.Version)
	}
	return b
}

func checkList(t *testing.T, what string, list []string, domains bool) {
	t.Helper()
	if !sort.StringsAreSorted(list) {
		t.Errorf("%s is not sorted", what)
	}
	seen := map[string]bool{}
	for _, d := range list {
		if seen[d] {
			t.Errorf("%s: duplicate %q", what, d)
		}
		seen[d] = true
		if d != strings.ToLower(strings.TrimSpace(d)) {
			t.Errorf("%s: %q is not lower-case/trimmed", what, d)
		}
		if domains {
			if n, err := normDomain(d); err != nil || n != d {
				t.Errorf("%s: %q is not a normalised bare domain (%v)", what, d, err)
			}
		}
	}
}

func TestRulesFileWellFormed(t *testing.T) {
	b := loadBundled(t)
	total := 0
	owner := map[string][]string{}
	for _, cat := range Categories {
		r, ok := b.Categories[cat]
		if !ok {
			t.Fatalf("category %s missing", cat)
		}
		checkList(t, cat+" domains", r.Domains, true)
		checkList(t, cat+" keywords", r.Keywords, false)
		if len(r.Domains) < 50 {
			t.Errorf("%s has only %d domains", cat, len(r.Domains))
		}
		total += len(r.Domains)
		set := map[string]bool{}
		for _, d := range r.Domains {
			set[d] = true
			owner[d] = append(owner[d], cat)
		}
		// no entry is redundant because a parent domain is already listed
		for _, d := range r.Domains {
			for p := d; ; {
				i := strings.IndexByte(p, '.')
				if i < 0 {
					break
				}
				p = p[i+1:]
				if set[p] {
					t.Errorf("%s: %q is redundant, %q is listed", cat, d, p)
				}
			}
		}
		for _, k := range r.Keywords {
			if len(k) < minKeywordLen {
				t.Errorf("%s: keyword %q too short", cat, k)
			}
		}
	}
	if total < 400 {
		t.Errorf("only %d bundled domains in total", total)
	}
	for d, cats := range owner {
		if len(cats) > 1 && !intentionalOverlaps[d] {
			t.Errorf("%q is in several categories: %v", d, cats)
		}
	}
	for cat, r := range b.Allow {
		if bit(cat) == 0 {
			t.Errorf("allow: unknown category %q", cat)
		}
		if len(r.Keywords) != 0 {
			t.Errorf("allow %s: keywords not supported", cat)
		}
		checkList(t, "allow "+cat, r.Domains, true)
		// an allow entry must sit under, or at least not equal, something it exempts
		for _, d := range r.Domains {
			for _, listed := range b.Categories[cat].Domains {
				if d == listed {
					t.Errorf("allow %s: %q is also a rule", cat, d)
				}
			}
		}
	}
	t.Logf("bundled domains: %d total", total)
}

func TestBundledAllowEntriesAreUsed(t *testing.T) {
	// Every allow entry must exempt something: either a keyword hit or a
	// flagged parent domain. Otherwise it is dead weight.
	b := loadBundled(t)
	for cat, r := range b.Allow {
		for _, d := range r.Domains {
			parent := false
			for p := d; ; {
				i := strings.IndexByte(p, '.')
				if i < 0 {
					break
				}
				p = p[i+1:]
				for _, l := range b.Categories[cat].Domains {
					if l == p {
						parent = true
					}
				}
			}
			if !parent && len(b.Categories[cat].Keywords) == 0 {
				t.Errorf("allow %s: %q exempts nothing", cat, d)
			}
		}
	}
}

func TestWiderBundledMatches(t *testing.T) {
	tests := []struct {
		url  string
		want []string
	}{
		{"https://www.spankbang.com/", []string{Adult}},
		{"https://cdn.fansly.com/x", []string{Adult}},
		{"https://rule34.xxx/index.php", []string{Adult}},
		{"https://www.betfred.com/sports", []string{Gambling}},
		{"https://www.national-lottery.co.uk/", []string{Gambling}},
		{"https://m.leovegas.com/", []string{Gambling}},
		{"https://www.begambleaware.org/", []string{Gambling}},
		{"https://www.nhsinform.scot/", []string{Health}},
		{"https://www.cancerresearchuk.org/about-cancer", []string{Health}},
		{"https://www.samaritans.org/", []string{Health}},
		{"https://pharmacy2u.co.uk/", []string{Health}},
		{"https://app.babylonhealth.com/", nil},
		{"https://www.halifax.co.uk/", []string{Finance}},
		{"https://www.nationwide.co.uk/", []string{Finance}},
		{"https://online.hl.co.uk/", []string{Finance}},
		{"https://www.tax.service.gov.uk/personal-account", []string{Finance}},
		{"https://www.experian.co.uk/", []string{Finance}},
		{"https://www.commbank.com.au/", []string{Finance}},
		{"https://www.binance.us/", []string{Finance}},
		{"https://proton.me/mail", []string{Privacy}},
		{"https://www.privacyguides.org/", []string{Privacy}},
		{"https://airvpn.org/", []string{Privacy}},
		{"https://www.thetimes.co.uk/", []string{NewsPolitics}},
		{"https://news.sky.com/politics", []string{NewsPolitics}},
		{"https://www.ft.com/", []string{NewsPolitics}},
		{"https://www.lemonde.fr/", []string{NewsPolitics}},
		{"https://plaid.cymru/", []string{NewsPolitics}},
		{"https://www.opendemocracy.net/", []string{NewsPolitics}},
		{"https://hansard.parliament.uk/", []string{NewsPolitics}},
	}
	for _, tt := range tests {
		if got := Classify(tt.url); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v want %v", tt.url, got, tt.want)
		}
	}
}

func TestWiderLookalikesAndBenignNeighbours(t *testing.T) {
	for _, u := range []string{
		// lookalikes: suffix must be on a label boundary
		"https://notpornhub.com/",
		"https://pornhub.com.example/",
		"https://fakebetfair.com/",
		"https://mynhs.net/",
		"https://wise.com.au/",
		"https://likewise.com/",
		"https://xhalifax.co.uk/",
		"https://nordvpn.com.evil.example/",
		"https://example.com/?u=https://www.ft.com/",
		"https://example.com/nordvpn.com",
		// the bare public suffix is never a rule
		"https://example.gov.uk/",
		"https://service.gov.uk/",
		"https://www.gov.uk/",
		"https://example.org.uk/",
		// big shared platforms are not flagged
		"https://www.google.com/search?q=casino",
		"https://www.youtube.com/watch?v=abc",
		"https://www.reddit.com/r/golang/",
		"https://twitter.com/home",
		"https://www.facebook.com/",
		"https://www.amazon.co.uk/",
		"https://duckduckgo.com/",
		"https://medium.com/@x/post",
		"https://cloudfront.net/x",
		"https://www.cloudflare.com/",
		"https://news.ycombinator.com/",
		// anchored keywords: not a whole token
		"https://example.com/poker-hand-evaluator",
		"https://example.com/pokerface/",
		"https://pokerface.example/",
		"https://example.com/casinos",
		"https://example.com/vpnconfig",
		"https://pornography-research.example/",
		"https://example.com/xxxl",
		"https://example.com/symptoms/",
		"https://example.com/online-bankingless",
		"https://royalcasino.example/",
	} {
		if got := Classify(u); got != nil {
			t.Errorf("%s: unexpected %v", u, got)
		}
	}
}

func TestAnchoredKeywords(t *testing.T) {
	tests := []struct {
		url  string
		want []string
	}{
		{"https://poker-room.example/", []string{Gambling}},
		{"https://best.casino/", []string{Gambling}},
		{"https://www.casino.example/", []string{Gambling}},
		{"https://example.com/poker", []string{Gambling}},
		{"https://example.com/a/poker/b", []string{Gambling}},
		{"https://example.com/POKER/", []string{Gambling}},
		{"https://my-vpn.example/", []string{Privacy}},
		{"https://example.com/vpn/setup", []string{Privacy}},
		{"https://secure.online-banking.example/", []string{Finance}},
		{"https://onlinebanking.example/", []string{Finance}},
		{"https://symptom-checker.example/", []string{Health}},
		{"https://telehealth.example/", []string{Health}},
	}
	for _, tt := range tests {
		if got := Classify(tt.url); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v want %v", tt.url, got, tt.want)
		}
	}
}

func TestBundledAllow(t *testing.T) {
	tests := []struct {
		url  string
		want []string
	}{
		{"https://en.wikipedia.org/wiki/Poker", nil},
		{"https://en.wikipedia.org/wiki/Casino", nil},
		{"https://github.com/acme/vpn/blob/main/README.md", nil},
		{"https://stackoverflow.com/questions/1/vpn/", nil},
		{"https://jobs.nhs.uk/", nil},
		{"https://www.jobs.nhs.uk/x", nil},
		{"https://cooking.nytimes.com/recipes/1", nil},
		{"https://jobs.theguardian.com/", nil},
		{"https://developer.paypal.com/docs", nil},
		{"https://docs.kraken.com/api", nil},
		{"https://careers.draftkings.com/", nil},
		// the parent domains stay flagged
		{"https://www.nhs.uk/", []string{Health}},
		{"https://www.nytimes.com/", []string{NewsPolitics}},
		{"https://www.theguardian.com/", []string{NewsPolitics}},
		{"https://www.paypal.com/", []string{Finance}},
		{"https://www.kraken.com/", []string{Finance}},
		{"https://www.draftkings.com/", []string{Gambling}},
	}
	for _, tt := range tests {
		if got := Classify(tt.url); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v want %v", tt.url, got, tt.want)
		}
	}
	// bundled allow never overrides what the user adds
	c := userClassifier(t, `{"add":{"gambling":{"domains":["github.com"]},"privacy":{"keywords":["vpn-notes"]}}}`)
	if got := c.Classify("https://github.com/x"); !reflect.DeepEqual(got, []string{Gambling}) {
		t.Errorf("user add on allowed host: %v", got)
	}
	if got := c.Classify("https://en.wikipedia.org/wiki/vpn-notes"); !reflect.DeepEqual(got, []string{Privacy}) {
		t.Errorf("user keyword on allowed host: %v", got)
	}
	// user allow still works on top
	c = userClassifier(t, `{"allow":{"finance":{"domains":["paypal.com"]}}}`)
	if got := c.Classify("https://www.paypal.com/"); got != nil {
		t.Errorf("user allow: %v", got)
	}
}

func TestBundledAllowMalformed(t *testing.T) {
	// the bundled format rejects keywords in allow; tokenMatch edge cases
	if tokenMatch("", "", "vpn") || tokenMatch("a.b", "/", "vpn") {
		t.Error("empty match")
	}
	if !tokenMatch("vpn", "", "vpn") || !tokenMatch("a-vpn", "", "vpn") || tokenMatch("avpn", "", "vpn") {
		t.Error("host boundaries")
	}
	if tokenMatch("x", "/vpnx", "vpn") || !tokenMatch("x", "/vpn", "vpn") {
		t.Error("path boundaries")
	}
	if !tokenMatch("avpn.vpn", "", "vpn") {
		t.Error("later occurrence after a non-boundary one")
	}
}

func TestPerformanceBundled100k(t *testing.T) {
	c := Default()
	urls := make([]string, 100_000)
	hits := 0
	for i := range urls {
		switch i % 4 {
		case 0:
			urls[i] = fmt.Sprintf("https://site%d.example/path/%d/page?q=%d", i, i, i)
		case 1:
			urls[i] = fmt.Sprintf("https://sub%d.hl.co.uk/x/%d", i, i)
			hits++
		case 2:
			urls[i] = fmt.Sprintf("https://a.b.c.deep%d.example/%d/long/path/segments/poker-hand", i, i)
		default:
			urls[i] = fmt.Sprintf("https://www.example%d.com/casino/%d", i, i)
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
	// about one second without -race; the limit leaves room for -race
	if d > 3*time.Second {
		t.Errorf("too slow: %v", d)
	}
}
