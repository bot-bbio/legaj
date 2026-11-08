package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/bot-bbio/legaj/internal/proc"
)

// The release checklist's scan, scripts/scan-sentinels.ps1 (R4, R34), is run
// by hand after a real run on a clean machine. These tests run the real
// script on folders built here, so a script that can't run, or that looks
// for the wrong text, fails verify instead of the release.

// runScanScript runs the scan with the shared sentinels on data and webview.
func runScanScript(t *testing.T, data, webview string) (code int, out string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the scan is a Windows PowerShell script")
	}
	cmd := proc.Command(context.Background(), "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join("scripts", "scan-sentinels.ps1"),
		"-Key", sentinelKey, "-Email", sentinelEmail, "-Phone", sentinelPhone, "-ResumeLine", sentinelResume,
		"-DataDir", data, "-WebViewDir", webview)
	raw, err := cmd.CombinedOutput()
	out = string(raw)
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("running the scan script: %v", err)
	}
	if code == 2 && strings.Contains(out, "Close LeGaJ first") {
		t.Fatal("a process named legaj is running on this machine, and the scan refuses to run then; close LeGaJ and run the tests again")
	}
	return code, out
}

func writeScanFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// utf16le encodes s the way Chromium's LevelDB and Windows APIs often store text.
func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func TestR34_ScanScriptPassesWhereTheDataBelongs(t *testing.T) {
	data, webview := t.TempDir(), t.TempDir()
	// The database and the documents hold the profile on purpose.
	writeScanFile(t, filepath.Join(data, "legaj.db"), []byte("profile "+sentinelEmail+" "+sentinelPhone+" "+sentinelResume))
	writeScanFile(t, filepath.Join(data, "logs", "legaj.log"), []byte(`{"msg":"letter approved","app":7}`))
	writeScanFile(t, filepath.Join(webview, "EBWebView", "Default", "Preferences"), []byte(`{"zoom":1}`))

	code, out := runScanScript(t, data, webview)
	if code != 0 || !strings.Contains(out, "nothing found") {
		t.Fatalf("exit %d on a clean run; output:\n%s", code, out)
	}
}

func TestR34_ScanScriptFindsTheKeyAnywhereInTheDataFolder(t *testing.T) {
	data, webview := t.TempDir(), t.TempDir()
	writeScanFile(t, filepath.Join(data, "backups", "legaj-1.db"), []byte("x"+sentinelKey+"x"))

	code, out := runScanScript(t, data, webview)
	if code != 1 || !strings.Contains(out, "FOUND sentinel values") || !strings.Contains(out, `  API key: `) || !strings.Contains(out, `backups\legaj-1.db`) {
		t.Fatalf("exit %d; the key in a backup was not reported; output:\n%s", code, out)
	}
}

func TestR34_ScanScriptFindsPersonalDataInLogsAndWebView(t *testing.T) {
	data, webview := t.TempDir(), t.TempDir()
	writeScanFile(t, filepath.Join(data, "logs", "legaj.log"), []byte(`{"msg":"parse","text":"`+sentinelEmail+`"}`))
	writeScanFile(t, filepath.Join(webview, "EBWebView", "Default", "Local Storage", "leveldb", "000003.log"), utf16le(sentinelPhone))

	code, out := runScanScript(t, data, webview)
	for _, want := range []string{"FOUND sentinel values", `  email: `, `logs\legaj.log`, `  phone: `, `leveldb\000003.log`} {
		if code != 1 || !strings.Contains(out, want) {
			t.Fatalf("exit %d; want %q in the output:\n%s", code, want, out)
		}
	}
	// Hits are listed as "  <what>: <path>"; the headers name every value too.
	if strings.Contains(out, "  resume line: ") || strings.Contains(out, "  API key: ") {
		t.Fatalf("reported a value that isn't there:\n%s", out)
	}
}
