// Command chrome-history serves a local UI for browsing Chromium history.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"chrome-history/internal/classify"
	"chrome-history/internal/history"
	"chrome-history/internal/server"
	"chrome-history/web"
)

func main() {
	profile := flag.String("profile", "", "path to a profile dir, user-data dir, or History file (default: auto-discover)")
	port := flag.Int("port", 8080, "TCP port on 127.0.0.1 (0 = random)")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	rules := flag.String("rules", "", "sensitive-site rules file (default: <user config dir>/chrome-history/rules.json, optional)")
	flag.Parse()

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

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := "http://" + ln.Addr().String() + "/"
	fmt.Printf("serving %d profile(s) at %s\n", len(profiles), addr)
	if !*noOpen {
		go openBrowser(addr)
	}
	srv := &http.Server{Handler: server.New(profiles, web.FS), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
