package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// getPythonPath resolves the Python executable, preferring the canonical
// Python 3.12 installation where project dependencies are installed.
func getPythonPath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		for _, ver := range []string{"Python312", "Python311", "Python310"} {
			p := filepath.Join(localAppData, "Programs", "Python", ver, "python.exe")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if path, err := exec.LookPath("python"); err == nil {
		return path
	}
	return "python"
}

// toolsBinaryPath locates the bundled "legaj-tools" executable produced by
// PyInstaller. In a packaged install the frozen binary lives next to the GUI
// executable (or in a sibling tools/ directory), so no Python runtime is
// required on the user's machine. Returns ("", false) when no frozen binary is
// found, in which case the caller falls back to running the .py scripts with a
// system Python interpreter (development mode).
func toolsBinaryPath() (string, bool) {
	exeName := "legaj-tools"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}

	var candidates []string
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		candidates = append(candidates,
			filepath.Join(dir, "tools", exeName),
			filepath.Join(dir, exeName),
		)
	}
	candidates = append(candidates, filepath.Join("tools", exeName))

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

// runPythonScript executes a tool with the given arguments and returns standard
// output. If the frozen "legaj-tools" binary is present it is invoked with the
// tool name as the first argument (e.g. "parse_resume"); otherwise the original
// .py script is run via the system Python interpreter.
func runPythonScript(scriptPath string, args ...string) (string, error) {
	var cmd *exec.Cmd
	if bin, ok := toolsBinaryPath(); ok {
		tool := strings.TrimSuffix(filepath.Base(scriptPath), ".py")
		cmd = exec.Command(bin, append([]string{tool}, args...)...)
	} else {
		fullArgs := append([]string{scriptPath}, args...)
		cmd = exec.Command(getPythonPath(), fullArgs...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running tool %s: %v (stderr: %s)", scriptPath, err, stderr.String())
	}

	return stdout.String(), nil
}

// RunParseResume calls parse_resume.py
func RunParseResume(resumePath string) (string, error) {
	return runPythonScript("scripts/parse_resume.py", resumePath)
}

// RunTailorResume calls tailor_resume.py
func RunTailorResume(basePath, tailoredPath string) (string, error) {
	return runPythonScript("scripts/tailor_resume.py", basePath, tailoredPath)
}

// RunGenerateResume calls generate_resume_pdf.py
func RunGenerateResume(profilePath, outputPath string) (string, error) {
	return runPythonScript("scripts/generate_resume_pdf.py", profilePath, outputPath)
}

// RunGenerateCoverLetter calls generate_cover_letter_pdf.py
func RunGenerateCoverLetter(profilePath, draftPath, outputPath string) (string, error) {
	return runPythonScript("scripts/generate_cover_letter_pdf.py", profilePath, draftPath, outputPath)
}

// RunPrepareInterview calls prepare_interview.py
func RunPrepareInterview(dataPath, mode string) (string, error) {
	return runPythonScript("scripts/prepare_interview.py", dataPath, mode)
}

// RunSearchJobs calls search_jobs.py
func RunSearchJobs(keywords, location string) (string, error) {
	return runPythonScript("scripts/search_jobs.py", keywords, location)
}

// RunManageApplications executes manage_applications.py
func RunManageApplications(action string, args ...string) (string, error) {
	fullArgs := append([]string{"references/job-tracker.json", action}, args...)
	return runPythonScript("scripts/manage_applications.py", fullArgs...)
}
