//go:build !windows

package ui

import "os"

// enableVT reports whether f can show ANSI colors. Unix terminals always can.
func enableVT(*os.File) bool { return true }

// OwnsConsoleWindow is always false outside Windows: terminals there stay open
// after a program exits.
func OwnsConsoleWindow() bool { return false }
