// Package files opens files for download confined to a base directory. It is the
// path-traversal guard behind the gated /files/* handlers: the request path
// comes straight from the URL, so it must never be able to escape its base.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrNotFound is returned when the requested file does not exist, is a
// directory, or the path escapes the base directory. All three collapse to one
// sentinel so callers answer 404 uniformly and never reveal whether a traversal
// was attempted versus a file merely being absent.
var ErrNotFound = errors.New("file not found")

// Open opens path under base for reading, confined to base. Confinement is
// enforced two ways: fs.ValidPath rejects "", absolute paths, and any ".."
// element up front, and os.Root is the backstop that also blocks symlink
// escapes. The caller owns the returned file and must Close it.
//
// Returns ErrNotFound for a missing file, a directory, or an escaping path;
// other (genuine IO) failures are returned wrapped.
func Open(base, path string) (*os.File, error) {
	// fs.ValidPath is the canonical check for an Open-able path: forward slashes,
	// not rooted, not empty, no ".." elements. Rejecting here means a malicious
	// path never reaches the filesystem at all.
	if !fs.ValidPath(path) || path == "." {
		return nil, ErrNotFound
	}

	root, err := os.OpenRoot(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound // base dir absent (e.g. a level with no files)
		}
		return nil, fmt.Errorf("open root %q: %w", base, err)
	}
	defer root.Close() // files opened from the root stay usable after Close

	// Any failure here — missing file or os.Root rejecting a symlink escape —
	// collapses to ErrNotFound.
	f, err := root.Open(path)
	if err != nil {
		return nil, ErrNotFound
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if info.IsDir() {
		f.Close()
		return nil, ErrNotFound // never list or serve a directory
	}

	return f, nil
}
