package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	// Layout:
	//   root/base/hello.txt          (the allowed file)
	//   root/base/sub/nested.txt     (allowed, nested)
	//   root/base/dir/               (a directory)
	//   root/secret.txt              (sibling, outside base)
	//   root/base/escape -> ../secret.txt  (symlink escaping base)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(filepath.Join(base, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(base, "hello.txt"), "hello")
	writeFile(t, filepath.Join(base, "sub", "nested.txt"), "nested")
	writeFile(t, filepath.Join(root, "secret.txt"), "top secret")
	if err := os.Symlink(filepath.Join("..", "secret.txt"), filepath.Join(base, "escape")); err != nil {
		t.Fatal(err)
	}

	t.Run("allowed file", func(t *testing.T) {
		assertContent(t, base, "hello.txt", "hello")
	})
	t.Run("nested allowed file", func(t *testing.T) {
		assertContent(t, base, "sub/nested.txt", "nested")
	})

	for _, tc := range []struct {
		name, path string
	}{
		{"parent traversal", "../secret.txt"},
		{"deep traversal", "sub/../../secret.txt"},
		{"absolute path", "/etc/passwd"},
		{"empty path", ""},
		{"dot path", "."},
		{"missing file", "nope.txt"},
		{"directory", "dir"},
		{"symlink escape", "escape"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Open(base, tc.path)
			if f != nil {
				f.Close()
			}
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("path %q: want ErrNotFound, got %v", tc.path, err)
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Run("lists files recursively, sorted, skipping dirs and dotfiles", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(base, "b.txt"), "b")
		writeFile(t, filepath.Join(base, "a.txt"), "a")
		writeFile(t, filepath.Join(base, "sub", "nested.txt"), "n")
		writeFile(t, filepath.Join(base, ".gitkeep"), "") // dotfile, must be skipped

		got, err := List(base)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		want := []string{"a.txt", "b.txt", "sub/nested.txt"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("missing base yields empty, non-nil slice", func(t *testing.T) {
		got, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("want empty non-nil slice, got %#v", got)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContent(t *testing.T, base, path, want string) {
	t.Helper()
	f, err := Open(base, path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("path %q: got %q, want %q", path, got, want)
	}
}
