package history

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TempPrefix is the name prefix of the private temp directories.
const TempPrefix = "chrome-history-"

var (
	liveMu sync.Mutex
	// liveTemp maps each live temp dir to a function that closes the database
	// opened on it (nil until the database exists). Windows cannot delete
	// files that are still open, so RemoveAllTemp must close them first.
	liveTemp = map[string]func(){}
)

func trackTemp(dir string) {
	liveMu.Lock()
	liveTemp[dir] = nil
	liveMu.Unlock()
}

func setTempCloser(dir string, closeDB func()) {
	liveMu.Lock()
	if _, ok := liveTemp[dir]; ok {
		liveTemp[dir] = closeDB
	}
	liveMu.Unlock()
}

func removeTemp(dir string) error {
	liveMu.Lock()
	delete(liveTemp, dir)
	liveMu.Unlock()
	return removeAllRetry(dir)
}

// removeAllRetry is os.RemoveAll with a few short retries on Windows, where
// deletion can transiently fail while a handle is being released.
func removeAllRetry(dir string) error {
	err := os.RemoveAll(dir)
	if runtime.GOOS != "windows" {
		return err
	}
	for i := 0; err != nil && i < 10; i++ {
		time.Sleep(20 * time.Millisecond)
		err = os.RemoveAll(dir)
	}
	return err
}

// RemoveAllTemp deletes every temp copy still owned by this process. Call it
// on shutdown after in-flight requests have finished or timed out.
func RemoveAllTemp() {
	liveMu.Lock()
	dirs := make([]string, 0, len(liveTemp))
	closers := make([]func(), 0, len(liveTemp))
	for d, c := range liveTemp {
		dirs = append(dirs, d)
		if c != nil {
			closers = append(closers, c)
		}
	}
	liveMu.Unlock()
	for _, c := range closers {
		c()
	}
	for _, d := range dirs {
		removeTemp(d)
	}
}

// SweepStale removes chrome-history-* directories in the system temp dir that
// were last modified more than maxAge ago and belong to the current user
// (left behind by a crash). It returns how many were removed.
func SweepStale(maxAge time.Duration) int {
	return sweepStale(os.TempDir(), maxAge, time.Now())
}

func sweepStale(root string, maxAge time.Duration, now time.Time) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), TempPrefix) {
			continue
		}
		path := filepath.Join(root, e.Name())
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() || now.Sub(fi.ModTime()) < maxAge || !ownedByCurrentUser(fi) {
			continue
		}
		liveMu.Lock()
		_, live := liveTemp[path]
		liveMu.Unlock()
		if live {
			continue
		}
		if os.RemoveAll(path) == nil {
			n++
		}
	}
	return n
}
