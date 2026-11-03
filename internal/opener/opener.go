// Package opener is the only way LeGaJ opens a link or a file (R33).
//
// V1 opened links through a shell, so a URL like file:///x&cd>pwned.txt
// ran a second command (A8, C7). Here:
//   - only http, https and mailto URLs are opened, re-serialized after
//     parsing, with no user-info and no control characters;
//   - only files inside the data folder are opened, and only document
//     types LeGaJ produces (an .exe there is still refused);
//   - the target goes straight to ShellExecute, with no shell, so "&" and
//     ">" are just characters.
package opener

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/apphome"
)

// Launcher hands a validated target to the OS (ShellExecute on Windows).
type Launcher interface {
	Launch(target string) error
}

// maxURL allows long compose links (a follow-up body, percent-encoded).
const maxURL = 32 << 10

// openableExt are the file types LeGaJ writes or stores.
var openableExt = map[string]bool{".pdf": true, ".docx": true, ".ics": true, ".txt": true}

// Opener validates and opens links and data-folder files.
type Opener struct {
	dataDir string
	launch  Launcher
}

// New creates an opener confined to dataDir.
func New(dataDir string, l Launcher) *Opener {
	return &Opener{dataDir: dataDir, launch: l}
}

func badLink() error {
	return apperr.New(apperr.CodeInvalid, "LeGaJ only opens web links (https://) and email links (mailto:).")
}

func badFile() error {
	return apperr.New(apperr.CodeInvalid, "LeGaJ only opens its own documents from its data folder.")
}

// OpenURL opens an http, https or mailto link in the user's default app.
func (o *Opener) OpenURL(raw string) error {
	if raw == "" || len(raw) > maxURL || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return badLink()
	}
	u, err := url.Parse(raw) // lower-cases the scheme
	if err != nil || u.User != nil {
		return badLink()
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" || u.Opaque != "" {
			return badLink()
		}
	case "mailto":
		if u.Opaque == "" {
			return badLink()
		}
	default:
		return badLink()
	}
	return o.run(u.String())
}

// OpenFile opens an existing document inside the data folder.
func (o *Opener) OpenFile(path string) error {
	if !filepath.IsAbs(path) {
		return badFile()
	}
	path = filepath.Clean(path)
	if !apphome.Within(path, o.dataDir) || strings.EqualFold(path, filepath.Clean(o.dataDir)) {
		return badFile()
	}
	if !openableExt[strings.ToLower(filepath.Ext(path))] {
		return badFile()
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return apperr.New(apperr.CodeNotFound, "That file is no longer in LeGaJ's folder.")
	}
	return o.run(path)
}

// OpenFolder opens a folder inside the data folder in File Explorer, so the
// user can attach a letter on an employer's site (R22).
func (o *Opener) OpenFolder(path string) error {
	if !filepath.IsAbs(path) {
		return badFile()
	}
	path = filepath.Clean(path)
	if !apphome.Within(path, o.dataDir) || strings.EqualFold(path, filepath.Clean(o.dataDir)) {
		return badFile()
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return apperr.New(apperr.CodeNotFound, "That folder is no longer in LeGaJ's folder.")
	}
	return o.run(path)
}

func (o *Opener) run(target string) error {
	if err := o.launch.Launch(target); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "Windows couldn't open that. Check that a browser or PDF viewer is installed.")
	}
	return nil
}
