// Package files opens files for download confined to a base directory. It is the
// path-traversal guard behind the gated /files/* handlers: the request path
// comes straight from the URL, so it must never be able to escape its base.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// List returns the regular files under base as relative, forward-slash paths,
// sorted. Directories and dotfiles (e.g. .gitkeep) are skipped. A missing base
// directory yields an empty (non-nil) slice — a level may simply have no files —
// rather than an error. base is caller-trusted (built from a level id), so this
// is a plain walk, not the traversal-guarded Open path.
func List(base string) ([]string, error) {
	names := []string{} // non-nil so JSON encodes [] not null
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return fs.SkipAll // base absent: nothing to list
			}
			return walkErr
		}
		if d.IsDir() {
			// Skip hidden subdirectories, but never the base dir itself.
			if path != base && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil // skip dotfiles like .gitkeep
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", base, err)
	}
	sort.Strings(names)
	return names, nil
}
