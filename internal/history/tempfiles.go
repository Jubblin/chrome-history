package history

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TempPrefix is the name prefix of the private temp directories.
const TempPrefix = "chrome-history-"

var (
	liveMu   sync.Mutex
	liveTemp = map[string]struct{}{}
)

func trackTemp(dir string) {
	liveMu.Lock()
	liveTemp[dir] = struct{}{}
	liveMu.Unlock()
}

func removeTemp(dir string) error {
	liveMu.Lock()
	delete(liveTemp, dir)
	liveMu.Unlock()
	return os.RemoveAll(dir)
}

// RemoveAllTemp deletes every temp copy still owned by this process. Call it
// on shutdown after in-flight requests have finished or timed out.
func RemoveAllTemp() {
	liveMu.Lock()
	dirs := make([]string, 0, len(liveTemp))
	for d := range liveTemp {
		dirs = append(dirs, d)
	}
	liveMu.Unlock()
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
