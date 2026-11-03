package pdf

import (
	"os"
	"path/filepath"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// Fit renders the letter with the first preset, from standard to most
// compact, that keeps it on one page. If even the most compact layout
// overflows, it returns ErrDoesntFit: V1 silently produced two-page letters
// (A12); a cover letter must be one page.
func Fit(l Letter) ([]byte, Preset, error) {
	if err := checkPrintable(l); err != nil {
		return nil, Preset{}, err
	}
	for _, p := range Presets {
		data, err := Render(l, p)
		if err == nil {
			return data, p, nil
		}
		if !IsDoesntFit(err) {
			return nil, Preset{}, err
		}
	}
	return nil, Preset{}, ErrDoesntFit
}

// WriteLetter fits the letter and writes it to path atomically: to a temp
// file in the same folder, then renamed (C4). When the letter doesn't fit,
// or anything fails, no file is left behind.
func WriteLetter(l Letter, path string) (Preset, error) {
	data, preset, err := Fit(l)
	if err != nil {
		return Preset{}, err
	}
	if err := writeAtomic(path, data); err != nil {
		return Preset{}, apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the PDF.")
	}
	return preset, nil
}

// Save writes rendered PDF bytes to path atomically (temp file, then
// rename). documents renders with Fit before its transaction, so only this
// quick write happens while the database writer is held.
func Save(path string, data []byte) error {
	if err := writeAtomic(path, data); err != nil {
		return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't save the PDF.")
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".letter-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
