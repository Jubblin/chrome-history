package history

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOpenRefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	db := mkDB(t, real, false)
	seed(t, db)
	db.Close()

	link := filepath.Join(dir, "History")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Open(link); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("symlinked History: got %v", err)
	}

	hist := filepath.Join(dir, "p", "History")
	os.MkdirAll(filepath.Dir(hist), 0o755)
	os.Rename(real, hist)
	for _, suf := range []string{"-wal", "-journal"} {
		l := hist + suf
		if err := os.Symlink(hist, l); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(hist); !errors.Is(err, ErrNotRegular) {
			t.Fatalf("symlinked %s: got %v", suf, err)
		}
		os.Remove(l)
	}
	if err := os.Mkdir(hist+"-wal", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(hist); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory as -wal: got %v", err)
	}
	os.Remove(hist + "-wal")
	r, err := Open(hist)
	if err != nil {
		t.Fatalf("regular file must still open: %v", err)
	}
	r.Close()
}

func TestCopyIsUnlinkedAfterOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History")
	db := mkDB(t, p, true)
	defer db.Close()
	seed(t, db)
	r, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for _, suf := range []string{"", "-wal", "-journal"} {
			if _, err := os.Lstat(filepath.Join(r.tmpDir, "History"+suf)); !os.IsNotExist(err) {
				t.Fatalf("History%s copy still on disk (%v)", suf, err)
			}
		}
	}
	// Queries still work on the unlinked copy, repeatedly.
	for i := 0; i < 3; i++ {
		if n, err := r.Count(Filter{}); err != nil || n != 5 {
			t.Fatalf("count after unlink: %d %v", n, err)
		}
	}
	tmp := r.tmpDir
	r.Close()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp dir not removed on Close")
	}
}

func TestRemoveAllTemp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "History")
	db := mkDB(t, p, false)
	seed(t, db)
	db.Close()
	r1, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	RemoveAllTemp()
	// The databases must be closed first: Windows cannot delete open files.
	for _, r := range []*Reader{r1, r2} {
		if err := r.db.Ping(); err == nil {
			t.Fatal("database still open after RemoveAllTemp")
		}
	}
	for _, d := range []string{r1.tmpDir, r2.tmpDir} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("%s survived RemoveAllTemp", d)
		}
	}
}

func TestSweepStale(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, age time.Duration) string {
		d := filepath.Join(root, name)
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, "History"), []byte("x"), 0o600)
		old := time.Now().Add(-age)
		if err := os.Chtimes(d, old, old); err != nil {
			t.Fatal(err)
		}
		return d
	}
	stale := mk("chrome-history-stale", time.Hour)
	fresh := mk("chrome-history-fresh", time.Minute)
	other := mk("something-else", time.Hour)
	file := filepath.Join(root, "chrome-history-file")
	os.WriteFile(file, nil, 0o600)
	os.Chtimes(file, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))

	if n := sweepStale(root, 10*time.Minute, time.Now()); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	for path, want := range map[string]bool{stale: false, fresh: true, other: true, file: true} {
		_, err := os.Stat(path)
		if (err == nil) != want {
			t.Errorf("%s exists=%v want %v", filepath.Base(path), err == nil, want)
		}
	}
}
