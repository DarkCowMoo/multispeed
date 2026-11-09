//go:build !windows

package main

import "os"

// enableVT reports whether f can show ANSI colors. Unix terminals always can.
func enableVT(*os.File) bool { return true }

// ownsConsoleWindow is always false outside Windows: terminals there stay open
// after a program exits.
func ownsConsoleWindow() bool { return false }
