// Package apphome resolves LeGaJ's data folder: the one place the database,
// backups, documents and logs live (R2).
//
// The folder is $LEGAJ_HOME when set, otherwise %LOCALAPPDATA%\LeGaJ. All
// code that touches user data must get its paths from here, so tests can
// redirect it and the guard in internal/testenv can see where it points.
package apphome

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar overrides the data folder. Tests always set it (see testenv).
const EnvVar = "LEGAJ_HOME"

// folderName is the per-user folder under %LOCALAPPDATA%.
const folderName = "LeGaJ"

// Dir returns the absolute data folder the app uses. It does no I/O.
func Dir() (string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvVar)); v != "" {
		return filepath.Abs(v)
	}
	return DefaultDir()
}

// DefaultDir returns %LOCALAPPDATA%\LeGaJ, the real per-user data folder.
// It does no I/O.
func DefaultDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if base == "" {
		var err error
		if base, err = os.UserCacheDir(); err != nil {
			return "", errors.New("cannot locate the local app data folder")
		}
	}
	return filepath.Abs(filepath.Join(base, folderName))
}

// Ensure creates dir (and parents) if it doesn't exist. On Windows the
// folder inherits the per-user ACL of %LOCALAPPDATA%.
func Ensure(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// Within reports whether path is base itself or inside it. Comparison is
// case-insensitive, as Windows paths are, and resolves symlinks/junctions
// for parts that exist so a link can't disguise the real folder.
func Within(path, base string) bool {
	p, b := canonical(path), canonical(base)
	if strings.EqualFold(p, b) {
		return true
	}
	rel, err := filepath.Rel(b, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// canonical cleans p, lowercases it and resolves links on its longest
// existing prefix. Read-only: it only stats.
func canonical(p string) string {
	p = filepath.Clean(p)
	existing, rest := p, ""
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			p = filepath.Join(resolved, rest)
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	return strings.ToLower(filepath.Clean(p))
}
