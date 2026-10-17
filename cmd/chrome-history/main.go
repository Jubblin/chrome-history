// Command chrome-history serves a local UI for browsing Chromium history.
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

	"chrome-history/internal/classify"
	"chrome-history/internal/history"
	"chrome-history/internal/server"
	"chrome-history/web"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	profile := flag.String("profile", "", "path to a profile dir, user-data dir, or History file (default: auto-discover)")
	port := flag.Int("port", 8080, "TCP port on 127.0.0.1 (0 = random)")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	rules := flag.String("rules", "", "sensitive-site rules file (default: <user config dir>/chrome-history/rules.json, optional)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("chrome-history", version)
		return
	}

	cls, err := classify.Load(*rules)
	if err != nil {
		log.Fatalf("rules: %v", err)
	}
	history.SetDefaultClassifier(cls)

	var profiles []history.Profile
	if *profile != "" {
		profiles, err = history.ResolvePath(*profile)
	} else {
		profiles, err = history.DiscoverProfiles()
	}
	if err != nil {
		log.Fatalf("profiles: %v", err)
	}
	if len(profiles) == 0 {
		log.Fatal("no browser profiles found; use --profile PATH")
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
