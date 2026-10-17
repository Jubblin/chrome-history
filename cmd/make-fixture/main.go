// Command make-fixture writes a synthetic Chromium user-data dir (fake data only)
// so chrome-history can be tried without touching a real browser profile.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"chrome-history/internal/history"

	_ "modernc.org/sqlite"
)

var sites = []string{"example.com", "docs.example.org", "news.example.net", "shop.example.com",
	"video.example.org", "wiki.example.net", "mail.example.com", "code.example.org", "maps.example.net", "blog.example.com"}

// flaggedSites exercise every sensitive-site category using domains from the
// bundled rules (plus a keyword match). Only the host names are real; all
// paths and titles are synthetic.
var flaggedSites = []string{"www.bet365.com", "pokerstars.com", "lucky-casino.example", "nordvpn.com", "mullvad.net",
	"torproject.org", "www.nhs.uk", "webmd.com", "www.paypal.com", "monzo.com", "www.bbc.co.uk", "theguardian.com",
	"porn-example.example", "casino-vpn.example"}

func main() {
	out := flag.String("out", "fixture", "output user-data directory (created; must not already hold a History)")
	days := flag.Int("days", 90, "number of days of history ending today (UTC)")
	perDay := flag.Int("per-day", 40, "average visits per day")
	seed := flag.Int64("seed", 1, "random seed (same seed, same data shape)")
	flag.Parse()

	dir := filepath.Join(*out, "Default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	hp := filepath.Join(dir, "History")
	if _, err := os.Stat(hp); err == nil {
		log.Fatalf("%s already exists; remove it first", hp)
	}
	ls := `{"profile":{"info_cache":{"Default":{"name":"Fixture","user_name":"fixture@example.com"}}}}`
	if !json.Valid([]byte(ls)) {
		log.Fatal("bad Local State")
	}
	if err := os.WriteFile(filepath.Join(*out, "Local State"), []byte(ls), 0o644); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(hp))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for _, s := range []string{
		`CREATE TABLE urls(id INTEGER PRIMARY KEY, url TEXT, title TEXT, visit_count INTEGER DEFAULT 0, typed_count INTEGER DEFAULT 0, last_visit_time INTEGER DEFAULT 0, hidden INTEGER DEFAULT 0)`,
		`CREATE TABLE visits(id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER, from_visit INTEGER, transition INTEGER, segment_id INTEGER DEFAULT 0, visit_duration INTEGER DEFAULT 0)`,
	} {
		if _, err := db.Exec(s); err != nil {
			log.Fatal(err)
		}
	}
	r := rand.New(rand.NewSource(*seed))
	urlIDs := map[string]int{}
	tx, _ := db.Begin()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	vid, n := 0, 0
	for d := *days - 1; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		cnt := r.Intn(*perDay*2 + 1)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			cnt /= 2
		}
		for i := 0; i < cnt; {
			start := day.Add(time.Duration(8*3600+r.Intn(14*3600)) * time.Second)
			burst := 1 + r.Intn(12)
			for j := 0; j < burst && i < cnt; j, i = j+1, i+1 {
				site := sites[r.Intn(len(sites))]
				if r.Intn(12) == 0 {
					site = flaggedSites[r.Intn(len(flaggedSites))]
				}
				path := fmt.Sprintf("/page-%d", r.Intn(60))
				u := "https://" + site + path
				id, ok := urlIDs[u]
				if !ok {
					id = len(urlIDs) + 1
					urlIDs[u] = id
					tx.Exec(`INSERT INTO urls(id,url,title) VALUES(?,?,?)`, id, u, fmt.Sprintf("%s – Page %s", site, path[6:]))
				}
				start = start.Add(time.Duration(5+r.Intn(240)) * time.Second)
				if !start.Before(day.AddDate(0, 0, 1)) {
					break
				}
				vid++
				trans := []int{0, 0, 0, 1, 2, 8}[r.Intn(6)]
				tx.Exec(`INSERT INTO visits(id,url,visit_time,from_visit,transition,visit_duration) VALUES(?,?,?,?,?,?)`,
					vid, id, history.ToWebKit(start), 0, trans, int64(r.Intn(120))*1_000_000)
				n++
			}
		}
	}
	tx.Exec(`UPDATE urls SET visit_count=(SELECT COUNT(*) FROM visits WHERE url=urls.id)`)
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d synthetic visits to %s\n", n, hp)
}
