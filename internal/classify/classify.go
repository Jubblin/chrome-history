// Package classify flags URLs that belong to sensitive categories using local
// rules only. It is a heuristic: lists are short, conservative starter sets
// and will both miss sites and occasionally flag the wrong one. Users correct
// it with a rules file (see Load). Classification never touches the network
// and never logs URLs.
//
// Categories:
//
//	adult, gambling, health, finance, privacy, news_politics
//
// news_politics is the broadest and most subjective category: news outlets,
// political parties, campaigns, advocacy and activist organisations and
// government political sites. It is deliberately separate from the others.
// privacy covers sites for privacy tools (VPN providers, proxies, Tor); it is
// about the sites visited, not incognito mode, which Chrome does not record.
package classify

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Category names.
const (
	Adult        = "adult"
	Gambling     = "gambling"
	Health       = "health"
	Finance      = "finance"
	Privacy      = "privacy"
	NewsPolitics = "news_politics"
)

// Categories lists all categories in sorted order.
var Categories = []string{Adult, Finance, Gambling, Health, NewsPolitics, Privacy}

// minKeywordLen limits false positives from very short keywords.
const minKeywordLen = 3

//go:embed rules.json
var bundledJSON []byte

// CategoryRules is a set of domain and keyword rules for one category.
type CategoryRules struct {
	Domains  []string `json:"domains,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

// UserRules is the on-disk format of the user rules file:
//
//	{"add":   {"gambling": {"domains": ["x.example"], "keywords": ["lotto"]}},
//	 "allow": {"news_politics": {"domains": ["bbc.co.uk"]}}}
//
// add extends a category. allow removes matches from a category and wins over
// both bundled rules and the user's own add rules. Domain rules match the
// domain and all its subdomains. Keyword rules are case-insensitive substrings
// of the host or path (not the query string).
type UserRules struct {
	Add   map[string]CategoryRules `json:"add,omitempty"`
	Allow map[string]CategoryRules `json:"allow,omitempty"`
}

type bundledRules struct {
	Version    int                      `json:"version"`
	Categories map[string]CategoryRules `json:"categories"`
}

type catSet uint8

func bit(cat string) catSet {
	for i, c := range Categories {
		if c == cat {
			return 1 << uint(i)
		}
	}
	return 0
}

type keywordRule struct {
	kw   string
	mask catSet
}

// Classifier classifies URLs. It is immutable and safe for concurrent use.
type Classifier struct {
	domains       map[string]catSet // registrable-suffix -> categories
	keywords      []keywordRule
	allowDomains  map[string]catSet
	allowKeywords []keywordRule
}

var (
	defaultOnce sync.Once
	defaultC    *Classifier
)

// Default returns a classifier with only the bundled rules.
func Default() *Classifier {
	defaultOnce.Do(func() {
		c, err := newClassifier(nil)
		if err != nil {
			panic("classify: bundled rules invalid: " + err.Error())
		}
		defaultC = c
	})
	return defaultC
}

// Classify classifies rawURL with the bundled rules.
func Classify(rawURL string) []string { return Default().Classify(rawURL) }

// New builds a classifier from the bundled rules plus user rules (may be nil).
func New(user *UserRules) (*Classifier, error) { return newClassifier(user) }

func newClassifier(user *UserRules) (*Classifier, error) {
	var b bundledRules
	dec := json.NewDecoder(bytes.NewReader(bundledJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("bundled rules: %w", err)
	}
	c := &Classifier{domains: map[string]catSet{}, allowDomains: map[string]catSet{}}
	for cat, r := range b.Categories {
		if err := c.add(cat, r, false); err != nil {
			return nil, fmt.Errorf("bundled rules: %w", err)
		}
	}
	if user != nil {
		for cat, r := range user.Add {
			if err := c.add(cat, r, false); err != nil {
				return nil, fmt.Errorf("add: %w", err)
			}
		}
		for cat, r := range user.Allow {
			if err := c.add(cat, r, true); err != nil {
				return nil, fmt.Errorf("allow: %w", err)
			}
		}
	}
	return c, nil
}

func (c *Classifier) add(cat string, r CategoryRules, allow bool) error {
	m := bit(cat)
	if m == 0 {
		return fmt.Errorf("unknown category %q (valid: %s)", cat, strings.Join(Categories, ", "))
	}
	doms, kws := c.domains, &c.keywords
	if allow {
		doms, kws = c.allowDomains, &c.allowKeywords
	}
	for _, d := range r.Domains {
		n, err := normDomain(d)
		if err != nil {
			return fmt.Errorf("category %q: %w", cat, err)
		}
		doms[n] |= m
	}
	for _, k := range r.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if len(k) < minKeywordLen {
			return fmt.Errorf("category %q: keyword %q is shorter than %d characters", cat, k, minKeywordLen)
		}
		if strings.ContainsAny(k, " \t/?#") {
			return fmt.Errorf("category %q: keyword %q must not contain spaces or any of / ? #", cat, k)
		}
		*kws = append(*kws, keywordRule{k, m})
	}
	return nil
}

func normDomain(d string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(d))
	n = strings.TrimPrefix(n, "*.")
	n = strings.Trim(n, ".")
	if n == "" || strings.ContainsAny(n, " \t/:?#@*") || !strings.Contains(n, ".") {
		return "", fmt.Errorf("invalid domain %q (use a bare domain such as example.com)", d)
	}
	return n, nil
}

// Classify returns the sorted categories of rawURL; nil if none (or if the URL
// is not http(s)/ftp-like with a host). Pure: no network, no logging.
func (c *Classifier) Classify(rawURL string) []string {
	host, path, ok := hostPath(rawURL)
	if !ok {
		return nil
	}
	var got, allowed catSet
	for h := host; ; {
		got |= c.domains[h]
		allowed |= c.allowDomains[h]
		i := strings.IndexByte(h, '.')
		if i < 0 {
			break
		}
		h = h[i+1:]
	}
	if len(c.keywords) > 0 || len(c.allowKeywords) > 0 {
		hp := host + path
		for _, k := range c.keywords {
			if got&k.mask != k.mask && strings.Contains(hp, k.kw) {
				got |= k.mask
			}
		}
		if got != 0 {
			for _, k := range c.allowKeywords {
				if strings.Contains(hp, k.kw) {
					allowed |= k.mask
				}
			}
		}
	}
	got &^= allowed
	if got == 0 {
		return nil
	}
	var out []string
	for i, cat := range Categories {
		if got&(1<<uint(i)) != 0 {
			out = append(out, cat)
		}
	}
	return out
}

// hostPath extracts the lower-cased host and path of an absolute URL.
func hostPath(raw string) (string, string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ftp":
	default:
		return "", "", false
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if h == "" {
		return "", "", false
	}
	return h, strings.ToLower(u.EscapedPath()), true
}

// HasAny reports whether cats contains any of want (want empty = true).
func HasAny(cats, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, c := range cats {
		for _, w := range want {
			if c == w {
				return true
			}
		}
	}
	return false
}

// ParseCategories validates a list of category names (e.g. from a query string)
// and returns them sorted and de-duplicated.
func ParseCategories(names []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if bit(n) == 0 {
			return nil, fmt.Errorf("unknown category %q (valid: %s)", n, strings.Join(Categories, ", "))
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

// DefaultRulesPath returns <user config dir>/chrome-history/rules.json.
func DefaultRulesPath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "chrome-history", "rules.json"), nil
}

// ParseUserRules parses and validates user rules JSON. Unknown fields,
// unknown categories and malformed rules are errors.
func ParseUserRules(r io.Reader) (*UserRules, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var u UserRules
	if err := dec.Decode(&u); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON object")
	}
	if _, err := newClassifier(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Load builds a classifier from the bundled rules plus the rules file at path.
// With path == "" the default location is used and a missing file is fine.
// An explicit path must exist. Any invalid file returns an error naming the
// file; call it at startup so mistakes surface before serving requests.
func Load(path string) (*Classifier, error) {
	optional := path == ""
	if optional {
		p, err := DefaultRulesPath()
		if err != nil {
			return Default(), nil
		}
		path = p
	}
	f, err := os.Open(path)
	if err != nil {
		if optional && errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, fmt.Errorf("rules file %s: %w", path, err)
	}
	defer f.Close()
	u, err := ParseUserRules(f)
	if err != nil {
		return nil, fmt.Errorf("rules file %s: %w", path, err)
	}
	return New(u)
}
