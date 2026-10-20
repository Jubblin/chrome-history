// Command history-timeline is a browsing history audit tool: it serves a local timeline UI for Chromium-family browser history.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"history-timeline/internal/classify"
	"history-timeline/internal/history"
	"history-timeline/internal/server"
	"history-timeline/web"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	profile := flag.String("profile", "", "path to a profile dir, user-data dir, or History file (default: auto-discover)")
	port := flag.Int("port", 8080, "TCP port on 127.0.0.1 (0 = random)")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	rules := flag.String("rules", "", "sensitive-site rules file (default: <user config dir>/history-timeline/rules.json, optional)")
	list := flag.Bool("list-profiles", false, "print the discovered profiles (browser, name, visit count) and the directories searched, then exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("history-timeline", version)
		return
	}

	cls, err := classify.Load(*rules)
	if err != nil {
		log.Fatalf("rules: %v", err)
	}
	history.SetDefaultClassifier(cls)

	var profiles []history.Profile
	var report history.Report
	if *profile != "" {
		profiles, err = history.ResolvePath(*profile)
		if err != nil {
			log.Fatalf("profiles: %v", err)
		}
		report.Profiles = profiles
	} else {
		report = history.DiscoverReport()
		profiles = report.Profiles
	}
	if *list {
		listProfiles(report)
		return
	}
	if len(profiles) == 0 {
		fmt.Fprint(os.Stderr, "no browser profiles found; use --profile PATH\n"+report.Explain())
		os.Exit(1)
	}

	history.SweepStale(10 * time.Minute)

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	api := server.New(profiles, web.FS)
	// The only place the token is ever printed or logged.
	addr := "http://" + ln.Addr().String() + "/?token=" + api.Token()
	fmt.Printf("serving %d profile(s) at %s\n", len(profiles), addr)
	if !*noOpen {
		go openBrowser(addr)
	}
	srv := &http.Server{Handler: api, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			srv.Close()
		}
		history.RemoveAllTemp()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		history.RemoveAllTemp()
		os.Exit(1)
	}
	<-done
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser: %v (open %s manually)", err, u)
	}
}

// listProfiles prints one line per profile (no URLs, titles or emails) and the
// directories that were searched.
func listProfiles(rep history.Report) {
	if len(rep.Profiles) == 0 {
		fmt.Println("no profiles found")
	}
	for _, p := range rep.Profiles {
		count := "unreadable"
		if r, err := history.OpenProfile(p); err == nil {
			if n, err := r.Count(history.Filter{}); err == nil {
				count = strconv.Itoa(n) + " visits"
			}
			r.Close()
		} else if errors.Is(err, os.ErrPermission) {
			count = "permission denied"
		}
		label := p.Browser
		if p.Channel != "" {
			label += " " + p.Channel
		}
		fmt.Printf("%s / %s (%s): %s\n", label, p.Name, p.Kind, count)
	}
	if len(rep.Searched) > 0 {
		fmt.Print(rep.Explain())
	}
	history.RemoveAllTemp()
}
