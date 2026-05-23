package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed frontend/*
var frontendFS embed.FS

func main() {
	// Initialize directories
	os.MkdirAll("outputs", 0755)
	os.MkdirAll("references", 0755)

	// Subtree the embedded filesystem to serve static assets from the root path
	subFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		fmt.Printf("Error creating sub filesystem: %v\n", err)
		os.Exit(1)
	}

	// Handlers
	http.Handle("/", http.FileServer(http.FS(subFS)))
	http.HandleFunc("/api/parse-resume", handleParseResume)
	http.HandleFunc("/api/save-profile", handleSaveProfile)
	http.HandleFunc("/api/tailor-resume", handleTailorResume)
	http.HandleFunc("/api/generate-resume", handleGenerateResume)
	http.HandleFunc("/api/generate-cover-letter", handleGenerateCoverLetter)
	http.HandleFunc("/api/prep-interview", handlePrepInterview)
	http.HandleFunc("/api/search-jobs", handleSearchJobs)
	http.HandleFunc("/api/applications", handleApplications)

	// Launch default web browser
	url := "http://127.0.0.1:8080"
	fmt.Printf("Starting LeGaJ Dashboard Server at %s...\n", url)
	go openBrowser(url)

	err = http.ListenAndServe("127.0.0.1:8080", nil)
	if err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}

// openBrowser opens the system's default browser to the given URL
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}
}

// REST Handlers

func handleParseResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Handle multipart form upload
	err := r.ParseMultipartForm(10 << 20) // Max 10MB file
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("resume")
	if err != nil {
		http.Error(w, "Failed to parse file upload form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save temporary upload file
	os.MkdirAll("outputs", 0755)
	tempPath := filepath.Join("outputs", header.Filename)
	out, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "Failed to create local file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(w, "Failed to write file contents", http.StatusInternalServerError)
		return
	}

	// Process file through parse script
	outText, err := RunParseResume(tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up uploaded resume
	os.Remove(tempPath)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(outText))
}

func handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = os.WriteFile("references/user-profile.json", body, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}

func handleTailorResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		JobDescription string `json:"job_description"`
		TailoredJSON   string `json:"tailored_json"`
	}
	
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Save the tailored JSON profile to references/user-profile-tailored.json
	tailoredPath := "references/user-profile-tailored.json"
	err = os.WriteFile(tailoredPath, []byte(req.TailoredJSON), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Call visual diff comparison script
	out, err := RunTailorResume("references/user-profile.json", tailoredPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(out))
}

func handleGenerateResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	profileSource := "references/user-profile.json"
	if _, err := os.Stat("references/user-profile-tailored.json"); err == nil {
		profileSource = "references/user-profile-tailored.json"
	}

	outPath := "outputs/Resume_Generated.pdf"
	out, err := RunGenerateResume(profileSource, outPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(fmt.Sprintf("%s\nSaved to: %s", out, outPath)))
}

func handleGenerateCoverLetter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Company   string `json:"company"`
		Role      string `json:"role"`
		DraftText string `json:"draft_text"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tempDraftPath := "outputs/temp_cover_letter.txt"
	err = os.WriteFile(tempDraftPath, []byte(req.DraftText), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempDraftPath)

	compClean := strings.ReplaceAll(req.Company, " ", "_")
	outPath := fmt.Sprintf("outputs/%s_Cover_Letter.pdf", compClean)

	out, err := RunGenerateCoverLetter("references/user-profile.json", tempDraftPath, outPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(fmt.Sprintf("%s\nSaved to: %s", out, outPath)))
}

func handlePrepInterview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode string `json:"mode"` // "anki", "cheatsheet", or "all"
		Data string `json:"data"` // JSON data representing flashcards/cheatsheet details
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tempDataPath := "outputs/temp_prep_data.json"
	err = os.WriteFile(tempDataPath, []byte(req.Data), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempDataPath)

	out, err := RunPrepareInterview(tempDataPath, req.Mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(out))
}

func handleSearchJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Keywords string `json:"keywords"`
		Location string `json:"location"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	out, err := RunSearchJobs(req.Keywords, req.Location)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(out))
}

func handleApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// List tracked jobs from Excel tracker
		out, err := RunManageApplications("list")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(out))
		
	} else if r.Method == http.MethodPost {
		// Log a new application or trigger IMAP sync
		var req struct {
			Action       string `json:"action"` // "add", "sync", or "update"
			Company      string `json:"company"`
			Role         string `json:"role"`
			Location     string `json:"location"`
			Link         string `json:"link"`
			Status       string `json:"status"`
			Resume       string `json:"resume"`
			CoverLetter  string `json:"cover_letter"`
			Notes        string `json:"notes"`
			Email        string `json:"email"`
			Password     string `json:"password"`
			IMAPServer   string `json:"imap_server"`
		}

		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var out string
		if req.Action == "add" {
			out, err = RunManageApplications("add", req.Company, req.Role, req.Location, req.Link, req.Resume, req.CoverLetter, req.Notes)
		} else if req.Action == "update" {
			out, err = RunManageApplications("update", req.Company, req.Role, req.Status, req.Notes)
		} else if req.Action == "sync" {
			out, err = RunManageApplications("sync", req.Email, req.Password, req.IMAPServer)
		} else {
			http.Error(w, "Invalid application tracking action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(out))
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
