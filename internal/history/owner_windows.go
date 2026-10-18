//go:build windows

package history

import "io/fs"

// Windows temp dirs are per-user, so every match is the current user's.
func ownedByCurrentUser(fs.FileInfo) bool { return true }
