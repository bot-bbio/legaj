package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// runPythonScript executes a Python script with the given arguments and returns standard output
func runPythonScript(scriptPath string, args ...string) (string, error) {
	fullArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(getPythonPath(), fullArgs...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running script %s: %v (stderr: %s)", scriptPath, err, stderr.String())
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
