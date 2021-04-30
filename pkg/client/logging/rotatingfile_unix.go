// +build !windows

package logging

import (
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

// createFile creates a new file or truncates an existing file.
func createFile(path string, perm os.FileMode) (*os.File, error) {
	os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
}

// openForAppend opens a file for append or creates it if it doesn't exist.
func openForAppend(path string, perm os.FileMode) (*os.File, error) {
	os.OpenFile(logfilePath, os.O_WRONLY|os.O_APPEND, perm)
}

// IsTerminal returns whether the given file descriptor is a terminal
var IsTerminal = terminal.IsTerminal
