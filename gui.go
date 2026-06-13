package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"image/color"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	// clipCardTitle is the user-facing name of the job-leads inbox.
	clipCardTitle = "Job Leads"
	// linkedInCreditDisplay is the author credit shown on the Settings and Help tabs.
	linkedInCreditDisplay = "LinkedIn: Roberto Montero"
	// linkedInCreditURL is the profile linked by the author credit button.
	linkedInCreditURL = "https://linkedin.com/in/alvvays"
	// trackerColSelectWidth is the width of the Job Tracker's selection-checkbox
	// column. Kept compact so the checkbox isn't surrounded by dead space.
	trackerColSelectWidth = float32(40)
	// resumeTailoringEnabled gates the AI-driven resume tailoring feature
	// (Track & Tailor pipeline, Tailor Selected bulk action, wizard Step 4
	// strategy selector). The 1.0 alpha ships with this OFF because output
	// quality is not yet at a suitable level; the implementation is
	// intentionally retained for a future build. To re-enable, flip to true
	// and the original UI surfaces will re-appear without further changes.
	resumeTailoringEnabled = false
)

// getTailorModeOptions returns the choices offered by the "Tailor Selected"
// dialog, in display order.
func getTailorModeOptions() []string {
	return []string{"Resume", "Cover Letter", "Both"}
}

// wizardNavigator encapsulates step navigation for the setup wizard so the
// Skip-button behavior can be unit tested independently of the GUI.
type wizardNavigator struct {
	currentStep int
	totalSteps  int
}

// skip returns the step to advance to when the user presses Skip. For any step
// before the last it advances by one (shouldClose=false). On the final step it
// signals the wizard should close (nextStep=0, shouldClose=true).
func (wn *wizardNavigator) skip() (nextStep int, shouldClose bool) {
	if wn.currentStep >= wn.totalSteps {
		return 0, true
	}
	return wn.currentStep + 1, false
}

type PersonalInfo struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Location string `json:"location"`
	Linkedin string `json:"linkedin"`
	Website  string `json:"website"`
}

type Education struct {
	Institution    string `json:"institution"`
	Degree         string `json:"degree"`
	Major          string `json:"major"`
	GraduationDate string `json:"graduation_date"`
	Location       string `json:"location"`
	GPA            string `json:"gpa"`
	Details        string `json:"details"`
}

type Experience struct {
	Company   string   `json:"company"`
	Role      string   `json:"role"`
	Location  string   `json:"location"`
	StartDate string   `json:"start_date"`
	EndDate   string   `json:"end_date"`
	Bullets   []string `json:"bullets"`
}

type Project struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	Details      string   `json:"details"`
}

// ResumeSection captures any résumé section the parser intuits dynamically
// (e.g. Publications, Certifications, Awards). Standard sections map to typed
// fields above; anything the résumé actually contains beyond those lives here.
type ResumeSection struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

type Profile struct {
	PersonalInfo       PersonalInfo        `json:"personal_info"`
	TargetRoles        []string            `json:"target_roles"`
	Education          []Education         `json:"education"`
	Experience         []Experience        `json:"experience"`
	Projects           []Project           `json:"projects"`
	Skills             map[string][]string `json:"skills"`
	AdditionalSections []ResumeSection     `json:"additional_sections,omitempty"`
}

// geminiClient is the shared HTTP client for all Gemini API calls. The 30s
// timeout prevents a hung request from blocking the UI indefinitely (SEC-015).
var geminiClient = &http.Client{Timeout: 30 * time.Second}

// writeSecureFile writes files containing secrets or PII with owner-only
// permissions (SEC-002, SEC-031). Use for .env and references/*.json.
func writeSecureFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

// callGeminiOnce is a single, non-retrying call to the Gemini generateContent
// endpoint. It returns the extracted text, the HTTP status code (so callers
// can decide on retry policy), and any error. The status code is 0 when the
// failure is at the transport layer (e.g. network timeout) — those are also
// retryable.
func callGeminiOnce(apiKey, promptText string, isJson bool) (string, int, error) {
	model := state.ApiModel
	if model == "" {
		model = "gemini-3.1-flash-lite"
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)

	requestBody := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"parts": []interface{}{
					map[string]interface{}{
						"text": promptText,
					},
				},
			},
		},
	}

	if isJson {
		requestBody["generationConfig"] = map[string]interface{}{
			"responseMimeType": "application/json",
		}
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := geminiClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	err = json.Unmarshal(bodyBytes, &apiResp)
	if err != nil {
		return "", resp.StatusCode, err
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return "", resp.StatusCode, fmt.Errorf("empty response from Gemini API")
	}

	text := apiResp.Candidates[0].Content.Parts[0].Text
	if isJson {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text, resp.StatusCode, nil
}

// callGeminiGo wraps callGeminiOnce with retry/backoff for transient failures.
// Retryable: HTTP 429 (rate limit), 503 (overloaded), and transport-layer
// errors (status code 0). All other errors fail fast. Schedule: 4 attempts with
// 2s / 4s / 8s sleeps between them, capped at ~14s total wait.
func callGeminiGo(apiKey, promptText string, isJson bool) (string, error) {
	const maxAttempts = 4
	backoffs := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		text, status, err := callGeminiOnce(apiKey, promptText, isJson)
		if err == nil {
			return text, nil
		}
		lastErr = err
		retryable := status == http.StatusServiceUnavailable || status == http.StatusTooManyRequests || status == 0
		if !retryable || attempt == maxAttempts-1 {
			return "", err
		}
		time.Sleep(backoffs[attempt])
	}
	return "", lastErr
}

// GroundingSource is a real URL retrieved by Gemini during its web search.
type GroundingSource struct {
	URI   string
	Title string
}

// GroundedResponse contains both the model's text output and the actual
// sources Gemini fetched during search grounding — the sources are real URLs.
type GroundedResponse struct {
	Text    string
	Sources []GroundingSource
}

// callGeminiWithGrounding calls the Gemini API with google_search grounding enabled
// and returns both the text response AND the grounding chunk URLs (real verified sources).
func callGeminiWithGrounding(apiKey, promptText string) (GroundedResponse, error) {
	model := state.ApiModel
	if model == "" {
		model = "gemini-3.1-flash-lite"
	}
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)

	requestBody := map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{
				"parts": []interface{}{
					map[string]interface{}{
						"text": promptText,
					},
				},
			},
		},
		"tools": []interface{}{
			map[string]interface{}{
				"google_search": map[string]interface{}{},
			},
		},
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return GroundedResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return GroundedResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := geminiClient.Do(req)
	if err != nil {
		return GroundedResponse{}, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return GroundedResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return GroundedResponse{}, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Full response struct capturing grounding metadata (S1)
	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				GroundingChunks []struct {
					Web struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}

	err = json.Unmarshal(bodyBytes, &apiResp)
	if err != nil {
		return GroundedResponse{}, err
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return GroundedResponse{}, fmt.Errorf("empty response from Gemini API")
	}

	candidate := apiResp.Candidates[0]
	text := candidate.Content.Parts[0].Text
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var sources []GroundingSource
	for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
		if chunk.Web.URI != "" {
			sources = append(sources, GroundingSource{URI: chunk.Web.URI, Title: chunk.Web.Title})
		}
	}

	return GroundedResponse{Text: text, Sources: sources}, nil
}

// Whitelist and Blacklist of job boards.
// Whitelisted sites are known to have easily accessible job descriptions.
// Blacklisted sites require login, use heavy anti-bot walls, or don't host individual job postings.
var jobBoardWhitelist = []string{
	"greenhouse.io", "lever.co", "myworkdayjobs.com", "workday.com",
	"ashbyhq.com", "icims.com", "smartrecruiters.com", "jobvite.com",
	"recruitee.com", "linkedin.com", "indeed.com", "dice.com",
	"glassdoor.com", "brassring.com", "taleo.net",
}

var jobBoardBlacklist = []string{
	"ziprecruiter.com", "simplyhired.com", "linkup.com", "jooble.org",
	"careerbuilder.com", "neuvoo.com", "lensa.com",
}

// jobDomainTier returns 1 (direct ATS/career page), 2 (quality aggregator),
// or 3 (search page / low quality). Lower is better. (S3)
func jobDomainTier(rawURL string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 3
	}
	host := strings.ToLower(u.Host)
	path := strings.ToLower(u.Path)

	// Check blacklist first
	for _, b := range jobBoardBlacklist {
		if strings.Contains(host, b) {
			return 3
		}
	}

	// Check whitelist for Tier 1 / 2 classification
	// Tier 1: direct ATS / company career pages with a specific job path
	tier1Hosts := []string{
		"greenhouse.io", "lever.co", "myworkdayjobs.com", "workday.com",
		"brassring.com", "taleo.net", "ashbyhq.com", "icims.com",
		"smartrecruiters.com", "jobvite.com", "recruitee.com",
	}
	for _, h := range tier1Hosts {
		if strings.Contains(host, h) {
			// Must have a path suggesting a specific posting, not just the root
			if len(path) > 5 {
				return 1
			}
		}
	}
	// Company career subdomains (e.g. careers.google.com, jobs.apple.com)
	if strings.HasPrefix(host, "careers.") || strings.HasPrefix(host, "jobs.") {
		if len(path) > 5 {
			return 1
		}
	}

	// Tier 2: quality aggregators with a direct listing path
	if strings.Contains(host, "linkedin.com") && strings.Contains(path, "/view/") {
		return 2
	}
	if strings.Contains(host, "indeed.com") && strings.Contains(path, "/viewjob") {
		return 2
	}
	if strings.Contains(host, "glassdoor.com") && strings.Contains(path, "/job-listing") {
		return 2
	}
	if strings.Contains(host, "dice.com") && strings.Contains(path, "/job-detail") {
		return 2
	}
	if strings.Contains(host, "monster.com") && strings.Contains(path, "/job-openings") {
		return 2
	}

	// Tier 3: search pages and low-quality sources
	tier3Patterns := []string{
		"google.com/search",
		"linkedin.com/jobs/search",
		"indeed.com/jobs",
		"ziprecruiter.com/jobs-search",
		"glassdoor.com/Job/jobs",
	}
	for _, p := range tier3Patterns {
		if strings.Contains(rawURL, p) {
			return 3
		}
	}

	// Check if host is part of whitelist
	isWhitelisted := false
	for _, w := range jobBoardWhitelist {
		if strings.Contains(host, w) {
			isWhitelisted = true
			break
		}
	}

	// Default: unknown domain — treat as Tier 2 if path is specific and not blacklisted/low quality
	if len(path) > 10 && (isWhitelisted || !strings.Contains(host, "search")) {
		return 2
	}
	return 3
}

// sourceBadge returns a short human-readable source label for a URL. (S6)
func sourceBadge(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "Web"
	}
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "linkedin.com"):
		return "LinkedIn"
	case strings.Contains(host, "indeed.com"):
		return "Indeed"
	case strings.Contains(host, "greenhouse.io"):
		return "Greenhouse"
	case strings.Contains(host, "lever.co"):
		return "Lever"
	case strings.Contains(host, "glassdoor.com"):
		return "Glassdoor"
	case strings.Contains(host, "workday.com"), strings.Contains(host, "myworkdayjobs.com"):
		return "Workday"
	case strings.Contains(host, "ashbyhq.com"):
		return "Ashby"
	case strings.Contains(host, "dice.com"):
		return "Dice"
	case strings.Contains(host, "monster.com"):
		return "Monster"
	case strings.Contains(host, "ziprecruiter.com"):
		return "ZipRecruiter"
	case strings.Contains(host, "icims.com"):
		return "iCIMS"
	case strings.HasPrefix(host, "careers."), strings.HasPrefix(host, "jobs."):
		return "Company Site"
	default:
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			return strings.Title(parts[len(parts)-2])
		}
		return "Web"
	}
}

// closedJobSignals are phrases indicating a job is no longer available. (S2)
var closedJobSignals = []string{
	"job has been filled",
	"position has been filled",
	"no longer accepting",
	"position has been closed",
	"job is no longer available",
	"listing has expired",
	"this job has expired",
	"posting has been removed",
	"application period has closed",
	"this position is no longer",
	"job no longer available",
	"posting is no longer active",
	"we are no longer accepting applications",
	"this listing is closed",
	"unfortunately, this job",
}

// verifyJobLink checks if a URL is reachable AND the page content doesn't
// indicate the position is closed. Returns (alive, sourceVerified). (S2)
func verifyJobLink(rawURL string, groundingSources []GroundingSource) (alive bool, groundingVerified bool) {
	if !strings.HasPrefix(rawURL, "http") {
		return false, false
	}

	// Check if this link came directly from Gemini's grounding sources (S1)
	for _, src := range groundingSources {
		if strings.EqualFold(src.URI, rawURL) {
			groundingVerified = true
			break
		}
		// Also match if the base URLs are the same (ignore query params)
		srcBase := strings.Split(src.URI, "?")[0]
		linkBase := strings.Split(rawURL, "?")[0]
		if strings.EqualFold(srcBase, linkBase) {
			groundingVerified = true
			break
		}
	}

	httpClient := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return false, groundingVerified
	}

	// Browser-like headers to bypass simple anti-bot blocking (multiple verification means)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, groundingVerified
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return false, groundingVerified
	}

	// Check if final URL redirected to a domain root (sign of a deleted listing) (S2)
	finalURL := resp.Request.URL.String()
	finalPath := resp.Request.URL.Path
	if len(finalPath) <= 1 || finalPath == "/" {
		return false, groundingVerified
	}
	_ = finalURL

	// Read first 12KB and scan for closed-job signals (S2)
	limitedBody := io.LimitReader(resp.Body, 12*1024)
	bodyBytes, readErr := io.ReadAll(limitedBody)
	if readErr == nil {
		bodyLower := strings.ToLower(string(bodyBytes))
		for _, signal := range closedJobSignals {
			if strings.Contains(bodyLower, signal) {
				return false, groundingVerified // Page exists but job is closed
			}
		}
	}

	return true, groundingVerified
}

// Kept for backward compatibility with other callers in the codebase.
func callGeminiWithSearchGo(apiKey, promptText string) (string, error) {
	gr, err := callGeminiWithGrounding(apiKey, promptText)
	return gr.Text, err
}

// AppState holds the application's global GUI state
type AppState struct {
	App               fyne.App
	Window            fyne.Window
	Profile           *Profile
	Applications      []JobApplication
	SelectedAppIdx    int
	ApiKey            string
	ApiModel          string
	TailoringStrategy string
	Email             string
	Password          string
	ImapServer        string
	SaveFolder        string

	// Dashboard widgets
	WishlistLabel    *widget.Label
	AppliedLabel     *widget.Label
	InterviewLabel   *widget.Label
	OfferLabel       *widget.Label
	RejectedLabel    *widget.Label
	RecentBox        *fyne.Container
	SearchResultsBox *fyne.Container
	ClipInboxBox     *fyne.Container

	// Profile widgets
	NameEntry       *widget.Entry
	EmailEntry      *widget.Entry
	PhoneEntry      *widget.Entry
	LocEntry        *widget.Entry
	LinkedinEntry   *widget.Entry
	WebsiteEntry    *widget.Entry
	RolesEntry      *widget.Entry
	TechSkillsEntry *widget.Entry
	PmSkillsEntry   *widget.Entry
	EduContainer        *fyne.Container
	ExpContainer        *fyne.Container
	ProjContainer       *fyne.Container
	AddlSectionsContainer *fyne.Container

	// Tracker widgets
	TrackerTable    *widget.Table
	TrackerSelected *JobApplication

	// Tailoring widgets
	TailorJobSelect *widget.Select
	TailorReqsEntry *widget.Entry
	OriginalPreview *widget.Label
	TailoredPreview *widget.Label
	TailorCompare   *fyne.Container

	// Prep widgets
	PrepJobSelect  *widget.Select
	PrepStatus     *widget.Label
	FlashcardBox   *fyne.Container
	CardQuestion   *widget.Label
	CardAnswer     *widget.Label
	CardIndicator  *widget.Label
	Flashcards     []Flashcard
	CurrentCardIdx int

	// Settings widgets
	SettingsApiKey     *widget.Entry
	SettingsApiModel   *widget.Select
	SettingsEmail      *widget.Entry
	SettingsPassword   *widget.Entry
	SettingsImapServer *widget.Entry
	SettingsSaveFolder *widget.Entry
}

type Flashcard struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type InterviewPrepData struct {
	CompanyProfile string      `json:"company_profile"`
	ElevatorPitch  string      `json:"elevator_pitch"`
	Achievements   []string    `json:"key_achievements"`
	QuestionsToAsk []string    `json:"questions_to_ask"`
	Flashcards     []Flashcard `json:"flashcards"`
}

type JobApplication struct {
	Company     string `json:"company"`
	Role        string `json:"role"`
	Location    string `json:"location"`
	Date        string `json:"date"`
	Link        string `json:"link"`
	Status      string `json:"status"`
	Resume      string `json:"resume"`
	CoverLetter string `json:"cover_letter"`
	Notes       string `json:"notes"`
}

// Global state instance
var state AppState
var isGUIMode bool = false
var activePort int = 8080
var clipAuthToken string
var clipListener net.Listener

var trackerSelectedRows = make(map[int]bool)

// clipSelectedRows tracks which Job Leads rows are checked for bulk migration,
// keyed by the clip's index in loadClippedJobs() order. clipInboxJobCount is the
// number of real clip rows currently rendered in state.ClipInboxBox (excludes
// the header, separator, and empty-state placeholder); it lets us strip the
// placeholder before appending the first real row.
var clipSelectedRows = make(map[int]bool)
var clipInboxJobCount int

type ClippedJob struct {
	Company      string `json:"company"`
	Role         string `json:"role"`
	Location     string `json:"location"`
	Link         string `json:"link"`
	Description  string `json:"description"`
	NeedsReview  bool   `json:"needs_review,omitempty"`
	ReviewReason string `json:"review_reason,omitempty"`
}

// logClipFailure appends a structured entry to references/clip-failures.log when
// the clipper falls back on company or role detection. The file is owner-only
// (0600) since it may contain scraped PII / company context. Failures are
// non-fatal: the clip is still saved (with NeedsReview=true) so the user can
// correct it in the inbox.
func logClipFailure(scrapedCompany, scrapedRole, finalCompany, finalRole, link, reason string) {
	os.MkdirAll("references", 0755)
	path := "references/clip-failures.log"
	entry := fmt.Sprintf("%s\treason=%q\tlink=%q\tscraped=(company=%q role=%q)\tfinal=(company=%q role=%q)\n",
		time.Now().Format(time.RFC3339), reason, link, scrapedCompany, scrapedRole, finalCompany, finalRole)
	existing, _ := os.ReadFile(path)
	_ = writeSecureFile(path, append(existing, []byte(entry)...))
}

func saveClippedJob(job ClippedJob) {
	os.MkdirAll("references", 0755)
	filePath := "references/clipped-jobs.json"
	var jobs []ClippedJob
	data, err := os.ReadFile(filePath)
	if err == nil {
		_ = json.Unmarshal(data, &jobs)
	}
	jobs = append(jobs, job)
	newData, err := json.MarshalIndent(jobs, "", "  ")
	if err == nil {
		_ = os.WriteFile(filePath, newData, 0644)
	}
}

func loadClippedJobs() []ClippedJob {
	filePath := "references/clipped-jobs.json"
	var jobs []ClippedJob
	data, err := os.ReadFile(filePath)
	if err == nil {
		_ = json.Unmarshal(data, &jobs)
	}
	return jobs
}

func isGenericRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	genericTitles := []string{
		"jobs at career page",
		"current openings",
		"apply now",
		"job posting",
		"job details",
		"careers",
		"jobs",
		"job search",
		"work with us",
		"career opportunities",
	}
	for _, gt := range genericTitles {
		if r == gt || strings.Contains(r, gt) {
			return true
		}
	}
	return false
}

func correctGenericRole(apiKey, description, fallbackCompany string) string {
	if apiKey == "" || description == "" {
		return "(Role not detected)"
	}
	prompt := fmt.Sprintf(`You are an expert recruiter helper. Extract the exact job role/title from the following job description. Output ONLY the role title (e.g., "Senior Software Engineer"), with no other text, markdown, or conversational intro/outro.

Job Description:
%s`, description)

	result, err := callGeminiGo(apiKey, prompt, false)
	if err == nil {
		cleaned := strings.TrimSpace(result)
		if cleaned != "" && len(cleaned) < 100 && !isGenericRole(cleaned) {
			return cleaned
		}
	}
	return "(Role not detected)"
}

// clipInboxHeader builds the bold column-header row for the Job Leads inbox.
// Column 0 is a blank spacer aligning with the per-row selection checkbox.
func clipInboxHeader() *fyne.Container {
	return container.New(&clipperRowLayout{},
		widget.NewLabel(""),
		widget.NewLabelWithStyle("Company", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Role", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Location", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Source", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Actions", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)
}

// resetClipInboxHeader replaces the inbox contents with just the header row and a
// separator, and resets the rendered-job counter to zero.
func resetClipInboxHeader() {
	if state.ClipInboxBox == nil {
		return
	}
	state.ClipInboxBox.Objects = []fyne.CanvasObject{
		clipInboxHeader(),
		widget.NewSeparator(),
	}
	clipInboxJobCount = 0
}

// showClipInboxPlaceholder resets the inbox to header + separator and appends the
// given empty-state message. Because the count is zero afterwards, the next
// addClippedJobToInboxUI call strips this placeholder before adding a real row —
// fixing the bug where "Inbox cleared." lingered as a ghost row.
func showClipInboxPlaceholder(msg string) {
	if state.ClipInboxBox == nil {
		return
	}
	resetClipInboxHeader()
	state.ClipInboxBox.Add(widget.NewLabelWithStyle(msg, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
}

func addClippedJobToInboxUI(job ClippedJob) {
	if state.ClipInboxBox == nil {
		return
	}

	idx := clipInboxJobCount // index in load order; keys clipSelectedRows
	c := job.Company
	ro := job.Role
	lo := job.Location
	li := job.Link
	bd := sourceBadge(li)
	needsReview := job.NeedsReview

	openBtn := widget.NewButtonWithIcon("View", theme.VisibilityIcon(), func() {
		openLink(li)
	})

	check := widget.NewCheck("", func(checked bool) {
		clipSelectedRows[idx] = checked
	})
	check.SetChecked(clipSelectedRows[idx])

	// Already-tracked clips can't be migrated again: disable the checkbox. Tracked
	// state is shown with a "✓" prefix on the company (mirroring the "⚠" review
	// marker) rather than a label crammed into the fixed-width Actions column,
	// which previously overflowed past the View button.
	existing := findApplicationByLink(li)
	if existing == nil {
		existing = findApplicationByCompanyAndRole(c, ro)
	}
	tracked := existing != nil
	if tracked {
		check.SetChecked(false)
		check.Disable()
		delete(clipSelectedRows, idx)
	}

	companyLabel := widget.NewLabel(c)
	switch {
	case tracked:
		companyLabel = widget.NewLabel("✓ " + c)
	case needsReview:
		companyLabel = widget.NewLabel("⚠ " + c)
		companyLabel.TextStyle = fyne.TextStyle{Bold: true}
	}

	row := container.New(&clipperRowLayout{},
		check,
		companyLabel,
		widget.NewLabel(ro),
		widget.NewLabel(lo),
		widget.NewLabel(bd),
		container.NewHBox(openBtn),
	)

	// Strip the empty-state placeholder before adding the first real row.
	if clipInboxJobCount == 0 && len(state.ClipInboxBox.Objects) > 2 {
		state.ClipInboxBox.Objects = state.ClipInboxBox.Objects[:2]
	}
	state.ClipInboxBox.Add(row)
	clipInboxJobCount++
}

// migrateClippedJobsToTracker adds the given clipped jobs to the tracker, skipping
// any that are already tracked (matched by link or company+role). It returns the
// number added and skipped. Pure data layer — no UI — so it is unit-testable.
func migrateClippedJobsToTracker(jobs []ClippedJob) (added, skipped int) {
	for _, j := range jobs {
		if findApplicationByLink(j.Link) != nil || findApplicationByCompanyAndRole(j.Company, j.Role) != nil {
			skipped++
			continue
		}
		resumeName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(j.Company, " ", "_"))
		coverName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(j.Company, " ", "_"))
		if addApplicationGo(j.Company, j.Role, j.Location, j.Link, "Applied", resumeName, coverName, "Clipped from browser.") == nil {
			added++
		} else {
			skipped++
		}
	}
	return added, skipped
}

func loadClippedJobsToInbox() {
	if state.ClipInboxBox == nil {
		return
	}

	// Note: clipSelectedRows is intentionally NOT reset here. Clips are only ever
	// appended (handler) or fully cleared (Clear Inbox), so load-order indices
	// stay stable across a re-render. Re-rendering after Select All / migration
	// must preserve the user's checkbox state. Explicit resets live in the Clear
	// Inbox / Clear Selection / post-migration paths.

	jobs := loadClippedJobs()
	if len(jobs) == 0 {
		showClipInboxPlaceholder("No clipped jobs yet. Use the bookmarklet on any job board to clip listings here.")
		state.ClipInboxBox.Refresh()
		return
	}

	resetClipInboxHeader()
	for _, job := range jobs {
		addClippedJobToInboxUI(job)
	}
	state.ClipInboxBox.Refresh()
}

func findClippedJobDescription(company, role, link string) string {
	jobs := loadClippedJobs()
	if link != "" {
		for _, j := range jobs {
			if j.Link == link && j.Description != "" {
				return j.Description
			}
		}
	}
	for _, j := range jobs {
		if strings.EqualFold(j.Company, company) && strings.EqualFold(j.Role, role) && j.Description != "" {
			return j.Description
		}
	}
	return ""
}

var roleKeywords = []string{
	"engineer", "developer", "manager", "analyst", "lead", "director", "designer",
	"specialist", "consultant", "architect", "intern", "qa", "tester", "scientist",
	"officer", "administrator", "support", "writer", "editor", "coordinator", "head",
	"vp", "president", "associate", "expert", "sr", "jr", "senior", "junior",
	"staff", "principal", "recruiter", "strategist", "fellow", "counsel", "advocate",
}

func containsRoleKeyword(s string) bool {
	sLower := strings.ToLower(s)
	for _, kw := range roleKeywords {
		if strings.Contains(sLower, kw) {
			return true
		}
	}
	return false
}

func extractCompanyFromTitle(title string) string {
	titleLower := strings.ToLower(title)
	if idx := strings.Index(titleLower, " at "); idx != -1 {
		comp := title[idx+4:]
		comp = titleSanitizerRegex.ReplaceAllString(comp, "")
		comp = strings.TrimSpace(comp)
		if comp != "" && len(comp) < 60 {
			return comp
		}
	}
	// Pipe / bullet separators are reliable "Role | Company" portal markers
	// and we accept any length on either side.
	if parts := strings.FieldsFunc(title, func(r rune) bool {
		return r == '|' || r == '•'
	}); len(parts) >= 2 {
		if comp := pickCompanySide(parts[0], parts[1], 50); comp != "" {
			return comp
		}
	}
	// Hyphens appear inside role titles all the time ("Sr. Client Solutions
	// Manager, Performance & Expansion - Marketing Solutions"), so only
	// split on a single hyphen AND require BOTH sides to be short. This
	// keeps clean cases like "Google - Senior Engineer" working but
	// rejects role strings with internal punctuation.
	if strings.Count(title, "-") == 1 {
		idx := strings.Index(title, "-")
		left := strings.TrimSpace(title[:idx])
		right := strings.TrimSpace(title[idx+1:])
		if len(left) > 0 && len(left) <= 30 && len(right) > 0 && len(right) <= 30 {
			if comp := pickCompanySide(left, right, 30); comp != "" {
				return comp
			}
		}
	}
	return ""
}

// pickCompanySide chooses the company half of a two-part title split. When one
// side contains a role keyword and the other does not, the other side wins;
// otherwise the leftmost side wins if it is within maxLen.
func pickCompanySide(left, right string, maxLen int) string {
	l := strings.TrimSpace(left)
	r := strings.TrimSpace(right)
	if containsRoleKeyword(l) && !containsRoleKeyword(r) {
		return r
	}
	if containsRoleKeyword(r) && !containsRoleKeyword(l) {
		return l
	}
	if len(l) < maxLen && l != "" {
		return l
	}
	return ""
}

// cleanText normalizes pasted/scraped text for safe display in Fyne widgets and
// storage. It converts CRLF/CR line endings to LF and strips non-printable
// control characters (which render as "?" boxes), while preserving newlines and
// tabs.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var sb strings.Builder
	for _, r := range s {
		if (r < 32 && r != '\n' && r != '\t') || r == 127 {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// stripRoleMetadata trims any trailing portal/req-id/location metadata after
// the first pipe character in a role title. Scraped roles like
// "Senior PM | Apple | Remote" contaminate cover-letter prompts; truncating
// at "|" keeps only the canonical role.
func stripRoleMetadata(role string) string {
	return strings.TrimSpace(strings.SplitN(role, "|", 2)[0])
}

func correctMissingCompany(apiKey, description, title string) string {
	if apiKey == "" || description == "" {
		return ""
	}
	prompt := fmt.Sprintf(`You are an expert recruiter helper. Extract the exact Company Name from the following job listing. Look at the title and description. Output ONLY the company name (e.g., "Google"), with no other text, markdown, or conversational intro/outro. If you cannot identify the company name, output "Unknown Company".

Job Title: %s
Job Description:
%s`, title, description)

	result, err := callGeminiGo(apiKey, prompt, false)
	if err == nil {
		cleaned := strings.TrimSpace(result)
		if cleaned != "" && len(cleaned) < 80 && !strings.Contains(strings.ToLower(cleaned), "unknown company") {
			return cleaned
		}
	}
	return ""
}

// genericCompanyTokens are values that legitimately look like a company string
// but almost always come from job-board page titles or navigation chrome rather
// than a real employer. When any of these is returned as the company, it should
// be treated as if the company is missing so the downstream fallback chain runs.
//
// Intentionally LIMITED to navigation chrome — major brands like LinkedIn,
// Google, Workday, etc. are removed because they are also real employers, and
// blocking them here causes false negatives for legitimate clips.
var genericCompanyTokens = []string{
	"jobs", "careers", "career", "apply", "apply now", "apply here",
	"job search", "job posting", "job postings", "job details",
	"job board", "job listings", "hiring", "we're hiring", "join us",
	"join our team", "open roles", "open positions", "current openings",
	"work with us", "career opportunities", "job opportunities",
	"all jobs", "search jobs", "find jobs", "view job",
}

// isGenericCompany returns true when the supplied company name is empty, looks
// like a URL or path fragment, is dominated by punctuation/digits, or matches a
// junk token from genericCompanyTokens. Mirrors isGenericRole for the company
// dimension so the /clip handler can drop noise before it reaches storage.
func isGenericCompany(name string) bool {
	c := strings.TrimSpace(name)
	if c == "" {
		return true
	}
	if strings.ContainsAny(c, "/\\") {
		return true
	}
	cLower := strings.ToLower(c)
	if strings.HasPrefix(cLower, "http://") || strings.HasPrefix(cLower, "https://") || strings.HasPrefix(cLower, "www.") {
		return true
	}
	if strings.Count(cLower, ".") > 0 && !strings.Contains(cLower, " ") {
		// Looks like a bare domain stem (e.g. "td.com") rather than a clean
		// brand name. A legitimate company with a dot (e.g. "Booking.com")
		// would still contain at least one space or be obviously brand-shaped
		// — the scraper rarely produces that as a false positive here.
		return true
	}
	if len(c) < 2 {
		return true
	}
	for _, tok := range genericCompanyTokens {
		if cLower == tok {
			return true
		}
	}
	return false
}

// extractCompanyHints pulls capitalized brand-like candidates from the opening
// of a job description. A majority of JDs name the employer in the first
// sentence ("About TD Bank...", "Acme is hiring...", "Join Stripe's growing
// team..."), so this gives a strong free signal before any LLM call. Returns
// up to 3 candidates, most-confident first; empty when the description doesn't
// open with anything obviously brand-shaped.
func extractCompanyHints(description string) []string {
	d := strings.TrimSpace(description)
	if d == "" {
		return nil
	}
	// Limit scan to the first sentence or the first ~200 chars, whichever is
	// shorter. Sentence terminators are ., !, ?, or a newline.
	head := d
	if len(head) > 200 {
		head = head[:200]
	}
	for i, r := range head {
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			head = head[:i]
			break
		}
	}
	words := strings.Fields(head)
	if len(words) > 30 {
		words = words[:30]
	}

	// Filler words that should not start or appear inside a captured brand
	// phrase even when capitalized (sentence-start or quirky styling).
	filler := map[string]bool{
		"a": true, "an": true, "the": true, "we": true, "our": true,
		"is": true, "are": true, "and": true, "or": true, "at": true,
		"in": true, "to": true, "of": true, "for": true, "with": true,
		"as": true, "by": true, "on": true, "from": true, "this": true,
		"that": true, "about": true, "join": true, "work": true,
		"role": true, "job": true, "position": true, "hiring": true,
	}

	isCapTok := func(w string) bool {
		w = strings.Trim(w, ",;:()[]\"'`")
		if w == "" {
			return false
		}
		first := rune(w[0])
		if !(first >= 'A' && first <= 'Z') {
			return false
		}
		if filler[strings.ToLower(w)] {
			return false
		}
		return true
	}

	cleanTok := func(w string) string {
		return strings.Trim(w, ",;:()[]\"'`.")
	}

	var hints []string
	seen := map[string]bool{}
	i := 0
	for i < len(words) {
		if !isCapTok(words[i]) {
			i++
			continue
		}
		// Greedy capture of contiguous capitalized tokens, allowing a single
		// lowercase glue word like "of" or "&" between two cap tokens.
		j := i
		var run []string
		for j < len(words) {
			if isCapTok(words[j]) {
				run = append(run, cleanTok(words[j]))
				j++
				continue
			}
			if j+1 < len(words) && (words[j] == "&" || filler[strings.ToLower(words[j])]) && isCapTok(words[j+1]) {
				run = append(run, words[j], cleanTok(words[j+1]))
				j += 2
				continue
			}
			break
		}
		if len(run) > 0 {
			cand := strings.TrimSpace(strings.Join(run, " "))
			cand = strings.Trim(cand, " &")
			key := strings.ToLower(cand)
			if len(cand) >= 2 && len(cand) < 60 && !seen[key] && !isGenericCompany(cand) {
				hints = append(hints, cand)
				seen[key] = true
			}
		}
		if j == i {
			i++
		} else {
			i = j
		}
		if len(hints) >= 3 {
			break
		}
	}
	return hints
}

// extractCompanyFromHost derives a brand candidate from a job-posting URL's
// hostname. Strips common subdomains (careers., jobs., apply., www.) and any
// known applicant-tracking-system suffix so that
// "careers.tdbank.com/job/123" → "tdbank", which the caller can capitalize or
// pass to an LLM for one-shot cleanup. Returns empty when the link is missing
// or only resolves to an ATS host.
func extractCompanyFromHost(link string) string {
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	// Strip a port if present.
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	// Strip common job-portal subdomains.
	for _, prefix := range []string{"www.", "careers.", "career.", "jobs.", "job.", "apply.", "hiring.", "talent.", "people.", "boards.", "board."} {
		host = strings.TrimPrefix(host, prefix)
	}
	// If the host is now a multi-employer job board (linkedin.com,
	// indeed.com) or an ATS that hosts roles for many brands
	// (greenhouse.io, lever.co), the path identifies the actual employer,
	// not the hostname. Return empty so the caller falls back to LLM
	// extraction from the description. NOTE: this list is for hostnames
	// only — the same names are still valid as employer values when they
	// appear in the description (LinkedIn, Workday, Google all hire).
	atsHosts := map[string]bool{
		"greenhouse.io": true, "grnh.se": true, "lever.co": true,
		"myworkdayjobs.com": true, "ashbyhq.com": true,
		"icims.com": true, "smartrecruiters.com": true, "bamboohr.com": true,
		"linkedin.com": true, "indeed.com": true, "glassdoor.com": true,
		"ziprecruiter.com": true,
	}
	if atsHosts[host] {
		return ""
	}
	// Drop the TLD ("tdbank.com" → "tdbank"). Multi-part TLDs like "co.uk"
	// would leave "tdbank.co" but that's still a reasonable LLM input.
	if idx := strings.LastIndex(host, "."); idx > 0 {
		host = host[:idx]
	}
	// "tdbank" is the stem; let the caller decide whether to titlecase it or
	// pipe it through correctMissingCompany for a brand-name cleanup.
	stem := strings.TrimSpace(host)
	if stem == "" || len(stem) < 2 || len(stem) > 40 {
		return ""
	}
	return stem
}

// brandFromHostStem capitalizes a hostname stem ("tdbank" → "Tdbank") for a
// readable display fallback. Kept deliberately simple — when an LLM is
// available, correctMissingCompany produces a cleaner result; this is the
// no-network fallback so the inbox never shows a bare lowercase stem.
func brandFromHostStem(stem string) string {
	if stem == "" {
		return ""
	}
	// Split on hyphens so "td-bank" → "Td Bank"; the LLM cleanup pass will
	// usually replace this with the canonical brand if one is reachable.
	parts := strings.Split(stem, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// companyAppearsInContext checks that the candidate company is referenced
// either by the description body or by the first-sentence hint list, using a
// case-insensitive token-containment match. Used after extraction to flag
// extractions that nothing in the source supports (often "Jobs", "Careers",
// or other page-chrome leaks) for user review.
func companyAppearsInContext(company, description string, hints []string) bool {
	c := strings.TrimSpace(company)
	if c == "" {
		return false
	}
	cLower := strings.ToLower(c)
	// Strip suffixes like ", Inc." / " LLC" so the match isn't defeated by a
	// punctuation tail that the description doesn't replicate verbatim.
	for _, suffix := range []string{", inc.", ", inc", " inc.", " inc", ", llc", " llc", ", ltd.", " ltd.", " ltd", ", corp.", " corp.", " corp"} {
		cLower = strings.TrimSuffix(cLower, suffix)
	}
	cLower = strings.TrimSpace(cLower)
	if cLower == "" {
		return false
	}
	if strings.Contains(strings.ToLower(description), cLower) {
		return true
	}
	for _, h := range hints {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(company)) {
			return true
		}
		if strings.Contains(strings.ToLower(h), cLower) || strings.Contains(cLower, strings.ToLower(h)) {
			return true
		}
	}
	return false
}

// extractJobDetailsLLM re-derives clean company, role, and location values from
// a scraped job description in a single LLM call, using the scraped values as
// hints. It returns ok=false when the API key or description is missing, or when
// the response cannot be parsed — callers should fall back to per-field
// heuristics in that case. This single pass replaces the separate
// correctMissingCompany/correctGenericRole calls in the common case.
func extractJobDetailsLLM(apiKey, scrapedCompany, scrapedRole, scrapedLocation, description string, hints []string) (company, role, location string, ok bool) {
	if apiKey == "" || strings.TrimSpace(description) == "" {
		return "", "", "", false
	}

	hintLine := "none"
	if len(hints) > 0 {
		hintLine = strings.Join(hints, " | ")
	}

	prompt := fmt.Sprintf(`You are an expert recruiter parsing agent. Read the following job description and scraped details and return the cleanest possible Company Name, Job Title (Role), and Job Location.

Scraped Company: %s
Scraped Role: %s
Scraped Location: %s

Likely company candidates from the description's first sentence (high signal — prefer these when the body is ambiguous): %s

Job Description:
%s

Guidance for the "company" field:
- The company is the actual hiring EMPLOYER. Major tech brands (Google, Apple, LinkedIn, Microsoft, Workday, etc.) ARE valid employers when they are doing the hiring — do not refuse to return them.
- Do NOT return navigation chrome as the company. Bad values include: "Jobs", "Careers", "Apply", "Apply Now", "Job Search", "Job Posting", "Hiring", "Open Roles", a bare URL, or a bare hostname (e.g. "tdbank.com").
- Prefer the Scraped Company when it is a clean brand name, then the first-sentence hints, then your reading of the description body. Only return an empty string for company when ALL signals look like chrome.

Output ONLY a valid JSON object matching this schema, with no markdown, no comments, and no other text:
{
  "company": "Clean Company Name (or empty string only as a last resort)",
  "role": "Clean Job Title / Role",
  "location": "Clean Location"
}`, scrapedCompany, scrapedRole, scrapedLocation, hintLine, description)

	result, err := callGeminiGo(apiKey, prompt, true)
	if err != nil {
		return "", "", "", false
	}

	var parsed struct {
		Company  string `json:"company"`
		Role     string `json:"role"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return "", "", "", false
	}

	return cleanText(strings.TrimSpace(parsed.Company)),
		cleanText(strings.TrimSpace(parsed.Role)),
		cleanText(strings.TrimSpace(parsed.Location)),
		true
}

func startFyneGUI() {
	initClipServer()
	state.App = app.NewWithID("com.legaj.desktop")
	state.App.Settings().SetTheme(customTheme{})
	state.Window = state.App.NewWindow("LeGaJ - Let's Get a Job Dashboard")
	state.Window.Resize(fyne.NewSize(1350, 750))

	// Ensure standard output/references directories
	os.MkdirAll("outputs", 0755)
	os.MkdirAll("references", 0755)

	// Load stored configurations (.env)
	loadConfigurations()

	// Initialize basic structures with empty values
	state.Profile = &Profile{
		PersonalInfo: PersonalInfo{Name: "", Email: "", Phone: "", Location: "", Linkedin: "", Website: ""},
		TargetRoles:  []string{},
		Education:    []Education{},
		Experience:   []Experience{},
		Projects:     []Project{},
		Skills:       map[string][]string{"technical": {}, "product_management": {}},
	}
	state.Applications = []JobApplication{}
	state.SelectedAppIdx = -1

	// Setup default save path if none
	if state.SaveFolder == "" {
		if _, err := os.Stat("outputs"); err == nil {
			state.SaveFolder = "outputs"
		} else {
			state.SaveFolder = "."
		}
	}

	// Initialize UI sections (build all tabs and widgets)
	dashboardTab := buildDashboardTab()
	jobHuntTab := buildJobHuntTab()
	profileTab := buildProfileTab()
	trackerTab := buildTrackerTab()
	fileManagerTab := buildFileManagerTab()
	settingsTab := buildSettingsTab()
	helpTab := buildHelpTab()
	// Tailor Assets and Interview Prep tabs are temporarily disabled pending
	// further testing. buildTailoringTab() and buildPrepTab() are intentionally
	// left defined so the features can be re-enabled in the future.

	// Arrange layout
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Dashboard", theme.HomeIcon(), dashboardTab),
		container.NewTabItemWithIcon("Job Hunt", theme.SearchIcon(), jobHuntTab),
		container.NewTabItemWithIcon("Base Profile", theme.AccountIcon(), profileTab),
		container.NewTabItemWithIcon("Job Tracker", theme.ListIcon(), trackerTab),
		container.NewTabItemWithIcon("File Manager", theme.FolderIcon(), fileManagerTab),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), settingsTab),
		container.NewTabItemWithIcon("Help", theme.HelpIcon(), helpTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	state.Window.SetContent(tabs)

	// Start the local Clip to LeGaJ bookmarklet listener
	startClipServer()

	// Load stored profile and Excel rows asynchronously
	go func() {
		loadProfileData()
		loadTrackerData()
		fyne.Do(func() {
			refreshUI()
			fillProfileForm()
			loadClippedJobsToInbox()

			// Onboarding Wizard triggers on startup if profile is uninitialized
			if forceWizard || state.Profile.PersonalInfo.Name == "" {
				showOnboardingWizard()
			}
		})
	}()

	state.Window.ShowAndRun()
}

// -------------------------------------------------------------
// Data Load/Save Helpers
// -------------------------------------------------------------

func findApplicationByCompanyAndRole(company, role string) *JobApplication {
	for i := range state.Applications {
		if strings.EqualFold(state.Applications[i].Company, company) && strings.EqualFold(state.Applications[i].Role, role) {
			return &state.Applications[i]
		}
	}
	return nil
}

func findApplicationByLink(link string) *JobApplication {
	if link == "" {
		return nil
	}
	for i := range state.Applications {
		if state.Applications[i].Link != "" && strings.EqualFold(state.Applications[i].Link, link) {
			return &state.Applications[i]
		}
	}
	return nil
}

func loadConfigurations() {
	state.SelectedAppIdx = -1
	state.ApiModel = "gemini-3.1-flash-lite" // default fallback
	state.TailoringStrategy = "job"          // default fallback
	bytes, err := os.ReadFile(".env")
	if err == nil {
		lines := strings.Split(string(bytes), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "GEMINI_API_KEY":
					state.ApiKey = val
				case "LEGAJ_API_MODEL":
					state.ApiModel = val
				case "LEGAJ_TAILORING_STRATEGY":
					state.TailoringStrategy = val
				case "LEGAJ_EMAIL":
					state.Email = val
				case "LEGAJ_PASSWORD":
					state.Password = val
				case "LEGAJ_IMAP_SERVER":
					state.ImapServer = val
				case "LEGAJ_SAVE_FOLDER":
					state.SaveFolder = val
				}
			}
		}
	}
}

func saveConfigurations() error {
	if state.ApiModel == "" {
		state.ApiModel = "gemini-3.1-flash-lite"
	}
	if state.TailoringStrategy == "" {
		state.TailoringStrategy = "job"
	}
	content := fmt.Sprintf("GEMINI_API_KEY=%s\nLEGAJ_API_MODEL=%s\nLEGAJ_TAILORING_STRATEGY=%s\nLEGAJ_EMAIL=%s\nLEGAJ_PASSWORD=%s\nLEGAJ_IMAP_SERVER=%s\nLEGAJ_SAVE_FOLDER=%s\n",
		state.ApiKey, state.ApiModel, state.TailoringStrategy, state.Email, state.Password, state.ImapServer, state.SaveFolder)
	return writeSecureFile(".env", []byte(content))
}

func loadProfileData() {
	bytes, err := os.ReadFile("references/user-profile.json")
	if err != nil {
		state.Profile = &Profile{
			PersonalInfo: PersonalInfo{Name: "", Email: "", Phone: "", Location: "", Linkedin: "", Website: ""},
			TargetRoles:  []string{},
			Education:    []Education{},
			Experience:   []Experience{},
			Projects:     []Project{},
			Skills:       map[string][]string{"technical": {}, "product_management": {}},
		}
		return
	}
	var prof Profile
	err = json.Unmarshal(bytes, &prof)
	if err != nil {
		fmt.Printf("Error unmarshaling profile JSON: %v\n", err)
		return
	}
	state.Profile = &prof
}

// sanitizeProfile strips CR/control characters from every string field of the
// profile in place. cleanText only removes invisible characters, so the visible
// text is unchanged; this guarantees nothing dirty is persisted regardless of
// whether the data came from the form, the wizard, or resume parsing.
func sanitizeProfile(p *Profile) {
	p.PersonalInfo.Name = cleanText(p.PersonalInfo.Name)
	p.PersonalInfo.Email = cleanText(p.PersonalInfo.Email)
	p.PersonalInfo.Phone = cleanText(p.PersonalInfo.Phone)
	p.PersonalInfo.Location = cleanText(p.PersonalInfo.Location)
	p.PersonalInfo.Linkedin = cleanText(p.PersonalInfo.Linkedin)
	p.PersonalInfo.Website = cleanText(p.PersonalInfo.Website)

	for i := range p.TargetRoles {
		p.TargetRoles[i] = cleanText(p.TargetRoles[i])
	}
	for i := range p.Education {
		e := &p.Education[i]
		e.Institution = cleanText(e.Institution)
		e.Degree = cleanText(e.Degree)
		e.Major = cleanText(e.Major)
		e.GraduationDate = cleanText(e.GraduationDate)
		e.Location = cleanText(e.Location)
		e.GPA = cleanText(e.GPA)
		e.Details = cleanText(e.Details)
	}
	for i := range p.Experience {
		ex := &p.Experience[i]
		ex.Company = cleanText(ex.Company)
		ex.Role = cleanText(ex.Role)
		ex.Location = cleanText(ex.Location)
		ex.StartDate = cleanText(ex.StartDate)
		ex.EndDate = cleanText(ex.EndDate)
		for j := range ex.Bullets {
			ex.Bullets[j] = cleanText(ex.Bullets[j])
		}
	}
	for i := range p.Projects {
		pr := &p.Projects[i]
		pr.Name = cleanText(pr.Name)
		pr.Description = cleanText(pr.Description)
		pr.Details = cleanText(pr.Details)
		for j := range pr.Technologies {
			pr.Technologies[j] = cleanText(pr.Technologies[j])
		}
	}
	for key, vals := range p.Skills {
		for i := range vals {
			vals[i] = cleanText(vals[i])
		}
		p.Skills[key] = vals
	}
	for i := range p.AdditionalSections {
		s := &p.AdditionalSections[i]
		s.Title = cleanText(s.Title)
		for j := range s.Items {
			s.Items[j] = cleanText(s.Items[j])
		}
	}
}

// validateTailoredProfile ensures a tailored profile JSON has not dropped or
// renamed top-level keys, shrunk core sections, or mutated skill categories
// relative to the base profile. Returns nil when safe to use, or an error
// describing what changed so callers can fall back to the base profile.
func validateTailoredProfile(baseJSON, tailoredJSON []byte) error {
	var base, tailored map[string]interface{}
	if err := json.Unmarshal(baseJSON, &base); err != nil {
		return fmt.Errorf("base profile invalid JSON: %v", err)
	}
	if err := json.Unmarshal(tailoredJSON, &tailored); err != nil {
		return fmt.Errorf("tailored profile invalid JSON: %v", err)
	}

	for key := range base {
		if _, ok := tailored[key]; !ok {
			return fmt.Errorf("tailored profile dropped top-level key %q", key)
		}
	}

	countSlice := func(m map[string]interface{}, key string) int {
		v, ok := m[key]
		if !ok || v == nil {
			return 0
		}
		s, ok := v.([]interface{})
		if !ok {
			return -1
		}
		return len(s)
	}
	for _, key := range []string{"experience", "education", "projects", "additional_sections"} {
		bc := countSlice(base, key)
		tc := countSlice(tailored, key)
		if bc > 0 && tc < bc {
			return fmt.Errorf("tailored profile shrunk %q from %d to %d entries", key, bc, tc)
		}
	}

	if baseSkills, ok := base["skills"].(map[string]interface{}); ok {
		tailoredSkills, _ := tailored["skills"].(map[string]interface{})
		for cat := range baseSkills {
			if _, ok := tailoredSkills[cat]; !ok {
				return fmt.Errorf("tailored profile dropped skills category %q", cat)
			}
		}
	}

	return nil
}

func saveProfileData() {
	if state.Profile == nil {
		return
	}
	sanitizeProfile(state.Profile)
	jsonData, err := json.MarshalIndent(state.Profile, "", "  ")
	if err != nil {
		dialog.ShowError(err, state.Window)
		return
	}
	err = writeSecureFile("references/user-profile.json", jsonData)
	if err != nil {
		dialog.ShowError(err, state.Window)
		return
	}
}

func saveTrackerDataGo() error {
	jsonData, err := json.MarshalIndent(state.Applications, "", "  ")
	if err != nil {
		return err
	}
	return writeSecureFile("references/job-tracker.json", jsonData)
}

func addApplicationGo(company, role, location, link, status, resume, coverLetter, notes string) error {
	dateStr := time.Now().Format("2006-01-02")
	if status == "" {
		status = "Applied"
	}
	newApp := JobApplication{
		Company:     cleanText(company),
		Role:        cleanText(role),
		Location:    cleanText(location),
		Date:        dateStr,
		Link:        cleanText(link),
		Status:      status,
		Resume:      resume,
		CoverLetter: coverLetter,
		Notes:       cleanText(notes),
	}
	state.Applications = append(state.Applications, newApp)
	return saveTrackerDataGo()
}

func updateApplicationGo(company, role, newStatus, notes string) error {
	notes = cleanText(notes)
	found := false
	for i := range state.Applications {
		if strings.EqualFold(strings.TrimSpace(state.Applications[i].Company), strings.TrimSpace(company)) &&
			strings.EqualFold(strings.TrimSpace(state.Applications[i].Role), strings.TrimSpace(role)) {
			state.Applications[i].Status = newStatus
			if notes != "" {
				currNotes := state.Applications[i].Notes
				if currNotes != "" {
					state.Applications[i].Notes = currNotes + " | " + notes
				} else {
					state.Applications[i].Notes = notes
				}
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no application found matching %s at %s", role, company)
	}
	return saveTrackerDataGo()
}

func deleteApplicationGo(company, role string) error {
	var newApps []JobApplication
	found := false
	for _, app := range state.Applications {
		if strings.EqualFold(strings.TrimSpace(app.Company), strings.TrimSpace(company)) &&
			strings.EqualFold(strings.TrimSpace(app.Role), strings.TrimSpace(role)) {
			found = true
		} else {
			newApps = append(newApps, app)
		}
	}
	if !found {
		return fmt.Errorf("no application found matching %s at %s", role, company)
	}
	state.Applications = newApps
	return saveTrackerDataGo()
}

func loadTrackerData() {
	filePath := "references/job-tracker.json"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Run python migration once if file does not exist
		_, _ = RunManageApplications("list")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		state.Applications = []JobApplication{}
		return
	}
	var apps []JobApplication
	err = json.Unmarshal(data, &apps)
	if err != nil {
		state.Applications = []JobApplication{}
		return
	}
	state.Applications = apps
}

// clearTrackerSelection resets all tracker selection state. trackerSelectedRows
// is keyed by row index, so after the Applications slice changes (add, import,
// migrate, delete) stale entries would otherwise map onto unrelated rows — the
// "allows selecting everything" bug. Centralizing the reset here fixes every
// mutation path at once.
func clearTrackerSelection() {
	for k := range trackerSelectedRows {
		delete(trackerSelectedRows, k)
	}
	state.SelectedAppIdx = -1
}

func reloadAllViews() {
	clearTrackerSelection()
	go func() {
		loadProfileData()
		loadTrackerData()
		fyne.Do(func() {
			refreshUI()
		})
	}()
}

func refreshUI() {
	updateDashboardStats()
	if state.TrackerTable != nil {
		state.TrackerTable.Refresh()
	}
	updateDropdownSelectors()
	updateTrackerSelectionUI()
}

func getSelectedTrackerIndices() []int {
	var indices []int
	for rowIdx, checked := range trackerSelectedRows {
		if checked && rowIdx < len(state.Applications) {
			indices = append(indices, rowIdx)
		}
	}
	if len(indices) == 0 && state.SelectedAppIdx >= 0 && state.SelectedAppIdx < len(state.Applications) {
		indices = append(indices, state.SelectedAppIdx)
	}
	return indices
}

func updateTrackerSelectionUI() {
	indices := getSelectedTrackerIndices()
	if len(indices) > 0 {
		// Set state.TrackerSelected to the first selected application for general details context
		firstIdx := indices[0]
		app := &state.Applications[firstIdx]
		state.TrackerSelected = app

		isUpdatingTrackerDropdown = true
		if trackerStatusSelect != nil {
			trackerStatusSelect.SetSelected(app.Status)
		}
		isUpdatingTrackerDropdown = false

		// The link check is a cheap string test, so compute it up front rather
		// than in the goroutine below (which exists for blocking disk stats).
		// Enable the Open Job URL button if ANY selected row has a valid link
		// (the click handler iterates all selected rows).
		hasLink := false
		for _, idx := range indices {
			if idx < 0 || idx >= len(state.Applications) {
				continue
			}
			l := strings.ToLower(state.Applications[idx].Link)
			if state.Applications[idx].Link != "" && (strings.HasPrefix(l, "http") || strings.HasPrefix(l, "file")) {
				hasLink = true
				break
			}
		}

		// Perform file stats check in a background goroutine to unblock UI thread
		go func() {
			hasResume := false
			hasCoverLetter := false
			for _, idx := range indices {
				if idx >= len(state.Applications) {
					continue
				}
				curApp := &state.Applications[idx]
				if curApp.Resume != "" {
					resPath := filepath.Join(state.SaveFolder, curApp.Resume)
					if _, err := os.Stat(resPath); err == nil {
						hasResume = true
					}
				}
				if curApp.CoverLetter != "" {
					clPath := filepath.Join(state.SaveFolder, curApp.CoverLetter)
					if _, err := os.Stat(clPath); err == nil {
						hasCoverLetter = true
					}
				}
			}

			// Update toolbar buttons on main thread
			fyne.Do(func() {
				// Verify if selection has not changed in the meantime
				currentIndices := getSelectedTrackerIndices()
				if len(currentIndices) == 0 {
					trackerOpenResumeBtn.Disable()
					trackerOpenCoverLetterBtn.Disable()
					trackerOpenLinkBtn.Disable()
					return
				}

				if hasResume {
					trackerOpenResumeBtn.Enable()
				} else {
					trackerOpenResumeBtn.Disable()
				}

				if hasCoverLetter {
					trackerOpenCoverLetterBtn.Enable()
				} else {
					trackerOpenCoverLetterBtn.Disable()
				}

				if hasLink {
					trackerOpenLinkBtn.Enable()
				} else {
					trackerOpenLinkBtn.Disable()
				}
			})
		}()
	} else {
		state.TrackerSelected = nil
		isUpdatingTrackerDropdown = true
		if trackerStatusSelect != nil {
			trackerStatusSelect.ClearSelected()
		}
		isUpdatingTrackerDropdown = false
		trackerOpenResumeBtn.Disable()
		trackerOpenCoverLetterBtn.Disable()
		trackerOpenLinkBtn.Disable()
	}
}

// resolveDocumentPath joins folder and filename and verifies the file exists on
// disk. It returns the joined path and true on success, or ("", false) if the
// filename is empty or the file cannot be stat'd.
func resolveDocumentPath(folder, filename string) (string, bool) {
	if strings.TrimSpace(filename) == "" {
		return "", false
	}
	full := filepath.Join(folder, filename)
	if _, err := os.Stat(full); err != nil {
		return "", false
	}
	return full, true
}

// openDocument resolves a saved document by filename and opens it with the OS
// default handler. It reports a friendly dialog if the file cannot be found.
func openDocument(filename, label string) {
	path, ok := resolveDocumentPath(state.SaveFolder, filename)
	if !ok {
		dialog.ShowInformation("File Not Found",
			fmt.Sprintf("Could not find the %s file. Make sure it has been generated and that your Save Folder in Settings is correct.", label),
			state.Window)
		return
	}
	openLink("file:///" + filepath.ToSlash(path))
}

func openLink(urlString string) {
	if strings.HasPrefix(urlString, "file://") {
		localPath := strings.TrimPrefix(urlString, "file://")
		if len(localPath) > 0 && localPath[0] == '/' {
			localPath = localPath[1:]
		}
		localPath = filepath.Clean(localPath)
		cmd := exec.Command("cmd", "/c", "start", "", localPath)
		cmd.Run()
		return
	}
	u, err := url.Parse(urlString)
	if err == nil {
		state.App.OpenURL(u)
	}
}

// -------------------------------------------------------------
// View Builders
// -------------------------------------------------------------

// 1. DASHBOARD VIEW WITH SEARCH GROUNDING
func buildDashboardTab() fyne.CanvasObject {
	state.WishlistLabel = widget.NewLabelWithStyle("0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	state.AppliedLabel = widget.NewLabelWithStyle("0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	state.InterviewLabel = widget.NewLabelWithStyle("0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	state.OfferLabel = widget.NewLabelWithStyle("0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	state.RejectedLabel = widget.NewLabelWithStyle("0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	kpiGrid := container.NewGridWithColumns(5,
		createKPICard("Wishlist", state.WishlistLabel, color.RGBA{R: 59, G: 130, B: 246, A: 255}),
		createKPICard("Applied", state.AppliedLabel, color.RGBA{R: 6, G: 182, B: 212, A: 255}),
		createKPICard("Interviewing", state.InterviewLabel, color.RGBA{R: 245, G: 158, B: 11, A: 255}),
		createKPICard("Offers Received", state.OfferLabel, color.RGBA{R: 16, G: 185, B: 129, A: 255}),
		createKPICard("Rejected", state.RejectedLabel, color.RGBA{R: 239, G: 68, B: 68, A: 255}),
	)

	state.RecentBox = container.NewVBox()
	recentScroll := container.NewVScroll(state.RecentBox)
	recentScroll.SetMinSize(fyne.NewSize(600, 240))

	recentCard := widget.NewCard("Recent Applications", "", recentScroll)

	dashboardSpacer := canvas.NewRectangle(color.Transparent)
	dashboardSpacer.SetMinSize(fyne.NewSize(16, 0))

	headerRow := container.NewHBox(
		canvas.NewText("Welcome to LeGaJ", theme.PrimaryColor()),
		layout.NewSpacer(),
		widget.NewButtonWithIcon("Run Onboarding Wizard", theme.DocumentCreateIcon(), func() {
			showOnboardingWizard()
		}),
		dashboardSpacer,
	)

	content := container.NewVBox(
		headerRow,
		widget.NewLabel("Let's Get a Job!"),
		widget.NewSeparator(),
		kpiGrid,
		recentCard,
	)

	return container.NewScroll(content)
}

func buildJobHuntTab() fyne.CanvasObject {
	// Job Discovery section using Gemini Search Grounding
	searchKeyword := widget.NewEntry()
	searchKeyword.SetPlaceHolder("e.g. Product Manager")
	searchLocation := widget.NewEntry()
	searchLocation.SetPlaceHolder("e.g. New York, NY")

	state.SearchResultsBox = container.NewVBox()
	resultsScroll := container.NewVScroll(state.SearchResultsBox)
	resultsScroll.SetMinSize(fyne.NewSize(600, 300))

	searchBtn := widget.NewButtonWithIcon("Find Jobs", theme.SearchIcon(), func() {
		if searchKeyword.Text == "" || searchLocation.Text == "" {
			dialog.ShowInformation("Required Info", "Please enter a job keyword and location.", state.Window)
			return
		}
		if state.ApiKey == "" {
			dialog.ShowInformation("API Key Missing", "Please configure your Gemini API Key in Settings first.", state.Window)
			return
		}

		kw := searchKeyword.Text
		loc := searchLocation.Text

		progress := dialog.NewProgressInfinite("Searching Active Jobs", "Querying Google via Gemini Search Grounding...", state.Window)
		progress.Show()

		go func() {
			// ── Strategy 4: Stronger prompt — direct posting URLs, recency, no fabrication ──
			prompt := fmt.Sprintf(`You are a job search assistant. Use your google_search tool RIGHT NOW to find real, currently active job postings.

Search for: "%s" jobs in "%s"

IMPORTANT RULES:
- Only return URLs you actually retrieved during this search. Do NOT invent or guess URLs.
- Each URL must be a direct link to a SPECIFIC job posting page, not a search results page.
- Prefer results posted within the last 30 days.
- Good URL patterns: /jobs/12345, /careers/view/, /job-application/, /postings/, /positions/
- BAD URLs (do not include): google.com/search, linkedin.com/jobs/search, indeed.com/jobs?q=, ziprecruiter.com/jobs-search
- Preferred sources (WHITELIST): Greenhouse, Lever, Workday, Ashby, iCIMS, LinkedIn (direct view links), Indeed (direct viewjob links), company career pages.
- Excluded sources (BLACKLIST): ZipRecruiter, SimplyHired, LinkUp, Jooble, CareerBuilder, Lensa. Do not return any listings from these sites.

Return a JSON array with exactly this structure, no other text:
[
  {
    "company": "Company Name",
    "role": "Exact Job Title",
    "location": "City, State or Remote",
    "link": "https://direct-posting-url.com/jobs/12345",
    "description": "One sentence on key skills required."
  }
]

Return ONLY valid JSON. No markdown, no explanation.`, kw, loc)

			groundedResp, apiErr := callGeminiWithGrounding(state.ApiKey, prompt)

			// Parse JSON — extract array regardless of markdown wrapping
			cleanJson := strings.TrimSpace(groundedResp.Text)
			if idx := strings.Index(cleanJson, "["); idx >= 0 {
				cleanJson = cleanJson[idx:]
			}
			if idx := strings.LastIndex(cleanJson, "]"); idx >= 0 {
				cleanJson = cleanJson[:idx+1]
			}

			type JobResult struct {
				Company     string `json:"company"`
				Role        string `json:"role"`
				Location    string `json:"location"`
				Link        string `json:"link"`
				Description string `json:"description"`
			}

			var results []JobResult
			parseErr := json.Unmarshal([]byte(cleanJson), &results)

			// ── Strategy 1: Also inject any grounding sources as candidate jobs ──
			// If Gemini returned grounding chunks that look like direct job listings,
			// add them as candidates (they're real URLs Gemini actually fetched).
			if parseErr == nil {
				groundingURLSet := make(map[string]bool)
				for _, r := range results {
					groundingURLSet[strings.ToLower(r.Link)] = true
				}
				for _, src := range groundedResp.Sources {
					if strings.ToLower(src.URI) == "" || groundingURLSet[strings.ToLower(src.URI)] {
						continue
					}
					// Only add if it looks like a direct job posting (Tier 1 or 2)
					if jobDomainTier(src.URI) <= 2 {
						results = append(results, JobResult{
							Company:     sourceBadge(src.URI),
							Role:        src.Title,
							Location:    loc,
							Link:        src.URI,
							Description: "Found via search grounding — click to view full posting.",
						})
						groundingURLSet[strings.ToLower(src.URI)] = true
					}
				}
			}

			// ── Strategy 3: Domain tier scoring — filter Tier 3 / search-page links ──
			type taggedResult struct {
				job               JobResult
				alive             bool
				groundingVerified bool
				tier              int
				badge             string
			}
			var tagged []taggedResult
			var searchPageLinks []JobResult // Tier 3 — relegated to fallback section

			if apiErr == nil && parseErr == nil && len(results) > 0 {
				verifyCh := make(chan taggedResult, len(results))
				for _, job := range results {
					j := job
					go func() {
						tier := jobDomainTier(j.Link)
						badge := sourceBadge(j.Link)
						if tier == 3 {
							// Demote to search-links section; skip deep verification
							verifyCh <- taggedResult{job: j, alive: false, tier: tier, badge: badge}
							return
						}
						// ── Strategy 2: Deep verification with body scan ──
						alive, groundingVerified := verifyJobLink(j.Link, groundedResp.Sources)
						verifyCh <- taggedResult{job: j, alive: alive, groundingVerified: groundingVerified, tier: tier, badge: badge}
					}()
				}
				for range results {
					t := <-verifyCh
					if t.tier == 3 {
						searchPageLinks = append(searchPageLinks, t.job)
					} else {
						tagged = append(tagged, t)
					}
				}
			}

			// All heavy work done — update UI on main thread
			fyne.Do(func() {
				progress.Hide()
				state.SearchResultsBox.Objects = nil

				if apiErr != nil {
					state.SearchResultsBox.Add(widget.NewLabelWithStyle(
						fmt.Sprintf("⚠️ Search Grounding limit/quota exceeded: %v", apiErr),
						fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
					))
					state.SearchResultsBox.Add(widget.NewLabel(
						"You can use these direct search links to browse active listings, then click your bookmarklet to clip them here:",
					))

					fallbackLinks := []struct{ name, link string }{
						{"LinkedIn Jobs", fmt.Sprintf("https://www.linkedin.com/jobs/search/?keywords=%s&location=%s", url.QueryEscape(kw), url.QueryEscape(loc))},
						{"Indeed", fmt.Sprintf("https://www.indeed.com/jobs?q=%s&l=%s", url.QueryEscape(kw), url.QueryEscape(loc))},
						{"Google Jobs", fmt.Sprintf("https://www.google.com/search?q=%s&ibp=htl;jobs", url.QueryEscape(kw+" jobs in "+loc))},
						{"Glassdoor", fmt.Sprintf("https://www.glassdoor.com/Job/jobs.htm?sc.keyword=%s&locT=C&locId=0", url.QueryEscape(kw))},
					}

					for _, fl := range fallbackLinks {
						flLink := fl.link
						flName := fl.name
						btn := widget.NewButtonWithIcon(flName, theme.SearchIcon(), func() {
							openLink(flLink)
						})
						state.SearchResultsBox.Add(btn)
					}
					state.SearchResultsBox.Refresh()
					return
				}

				verifiedCount := 0
				for _, t := range tagged {
					if t.alive {
						verifiedCount++
					}
				}

				if len(tagged) > 0 {
					summaryText := fmt.Sprintf("%d listing(s) found · %d verified active", len(tagged), verifiedCount)
					state.SearchResultsBox.Add(widget.NewLabelWithStyle(summaryText, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
					state.SearchResultsBox.Add(widget.NewSeparator())

					state.SearchResultsBox.Add(container.New(layout.NewGridLayout(5),
						widget.NewLabelWithStyle("Company", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						widget.NewLabelWithStyle("Role", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						widget.NewLabelWithStyle("Location", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						widget.NewLabelWithStyle("Source", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
						widget.NewLabelWithStyle("Actions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					))
					state.SearchResultsBox.Add(widget.NewSeparator())
				}

				// ── Strategy 6: Render job rows with source badge + verification status ──
				for _, t := range tagged {
					r := t.job
					existing := findApplicationByLink(r.Link)
					if existing == nil {
						existing = findApplicationByCompanyAndRole(r.Company, r.Role)
					}

					openBtn := widget.NewButtonWithIcon("View", theme.HelpIcon(), func() {
						openLink(r.Link)
					})
					trackTailorBtnLabel := "Track & Tailor"
					if !resumeTailoringEnabled {
						trackTailorBtnLabel = "Track & Apply"
					}
					trackTailorBtn := widget.NewButtonWithIcon(trackTailorBtnLabel, theme.DocumentCreateIcon(), func() {
						runTrackAndTailorAutomation(r.Company, r.Role, r.Location, r.Link, r.Description)
					})
					trackTailorBtn.Importance = widget.HighImportance

					if existing != nil {
						trackTailorBtn.SetText("Tracked (" + existing.Status + ")")
						trackTailorBtn.Disable()
					}

					sourceStr := t.badge
					if t.groundingVerified {
						sourceStr = "★ " + t.badge
					} else if t.alive {
						sourceStr = "✓ " + t.badge
					} else {
						sourceStr = "✗ " + t.badge
					}

					row := container.New(layout.NewGridLayout(5),
						widget.NewLabel(r.Company),
						widget.NewLabel(r.Role),
						widget.NewLabel(r.Location),
						widget.NewLabel(sourceStr),
						container.NewHBox(openBtn, trackTailorBtn),
					)
					state.SearchResultsBox.Add(row)
				}

				// ── Strategy 5: Fallback search links when no verified results ──
				if verifiedCount == 0 {
					state.SearchResultsBox.Add(widget.NewSeparator())
					state.SearchResultsBox.Add(widget.NewLabelWithStyle(
						"No verified postings found. Use these search links to continue manually:",
						fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
					))
					fallbackLinks := []struct{ name, link string }{
						{"LinkedIn Jobs", fmt.Sprintf("https://www.linkedin.com/jobs/search/?keywords=%s&location=%s", url.QueryEscape(kw), url.QueryEscape(loc))},
						{"Indeed", fmt.Sprintf("https://www.indeed.com/jobs?q=%s&l=%s", url.QueryEscape(kw), url.QueryEscape(loc))},
						{"Google Jobs", fmt.Sprintf("https://www.google.com/search?q=%s&ibp=htl;jobs", url.QueryEscape(kw+" jobs in "+loc))},
						{"Glassdoor", fmt.Sprintf("https://www.glassdoor.com/Job/jobs.htm?sc.keyword=%s&locT=C&locId=0", url.QueryEscape(kw))},
					}
					for _, fl := range fallbackLinks {
						flLink := fl.link
						flName := fl.name
						btn := widget.NewButtonWithIcon(flName, theme.SearchIcon(), func() {
							openLink(flLink)
						})
						state.SearchResultsBox.Add(btn)
						_ = flName
					}
				}

				if len(tagged) == 0 && len(searchPageLinks) == 0 && parseErr != nil {
					state.SearchResultsBox.Add(widget.NewLabel("Could not parse search results. Please try again."))
				}

				state.SearchResultsBox.Refresh()
			})
		}()
	})

	searchForm := container.New(layout.NewFormLayout(),
		widget.NewLabel("Keywords"), searchKeyword,
		widget.NewLabel("Location"), searchLocation,
	)

	searchCardContent := container.NewBorder(
		container.NewVBox(searchForm, searchBtn, widget.NewSeparator()),
		nil, nil, nil,
		resultsScroll,
	)
	searchTab := widget.NewCard("Job Discovery Engine", "", searchCardContent)

	// ── Job Leads Card (populated by the browser bookmarklet) ──
	state.ClipInboxBox = container.NewVBox()
	showClipInboxPlaceholder("No clipped jobs yet. Use the bookmarklet on any job board to clip listings here.")
	clipScroll := container.NewVScroll(state.ClipInboxBox)
	clipContainer := container.New(&fixedHeightLayout{height: 520}, clipScroll)

	clearClipBtn := widget.NewButtonWithIcon("Clear Inbox", theme.DeleteIcon(), func() {
		_ = os.Remove("references/clipped-jobs.json")
		clipSelectedRows = make(map[int]bool)
		showClipInboxPlaceholder("Inbox cleared.")
		state.ClipInboxBox.Refresh()
	})

	// Bulk migration: add every checked lead to the tracker in one action. Clips
	// are not removed — they re-render as disabled "Tracked (...)" rows, keeping
	// them available as the tailoring description cache.
	addSelectedBtn := widget.NewButtonWithIcon("Add Selected to Tracker", theme.ContentAddIcon(), func() {
		jobs := loadClippedJobs()
		var selected []ClippedJob
		var flagged []string
		for idx, checked := range clipSelectedRows {
			if checked && idx >= 0 && idx < len(jobs) {
				selected = append(selected, jobs[idx])
				if jobs[idx].NeedsReview {
					flagged = append(flagged, jobs[idx].Company)
				}
			}
		}
		if len(selected) == 0 {
			dialog.ShowInformation("Selection Required", "Check at least one clipped job to add to the tracker.", state.Window)
			return
		}

		doMigrate := func() {
			added, skipped := migrateClippedJobsToTracker(selected)
			clipSelectedRows = make(map[int]bool)
			reloadAllViews()
			loadClippedJobsToInbox() // re-render so migrated rows show as "Tracked"
			dialog.ShowInformation("Added to Tracker",
				fmt.Sprintf("%d job(s) added, %d skipped (already tracked).", added, skipped), state.Window)
		}

		if len(flagged) > 0 {
			msg := fmt.Sprintf("%d selected clip(s) were flagged for review (company/role may be incorrect): %s.\n\nAdd them to the tracker anyway?",
				len(flagged), strings.Join(flagged, ", "))
			dialog.ShowConfirm("Verify Job Details", msg, func(ok bool) {
				if ok {
					doMigrate()
				}
			}, state.Window)
			return
		}
		doMigrate()
	})
	addSelectedBtn.Importance = widget.HighImportance

	selectAllBtn := widget.NewButton("Select All", func() {
		jobs := loadClippedJobs()
		for idx, j := range jobs {
			// Only select clips that aren't already tracked.
			if findApplicationByLink(j.Link) == nil && findApplicationByCompanyAndRole(j.Company, j.Role) == nil {
				clipSelectedRows[idx] = true
			}
		}
		loadClippedJobsToInbox()
	})
	clearSelectionBtn := widget.NewButton("Clear Selection", func() {
		clipSelectedRows = make(map[int]bool)
		loadClippedJobsToInbox()
	})

	// TODO(2.0): restore the subheading "Jobs clipped from your browser via the
	// bookmarklet appear here for review" when the discovery engine ships.
	clipToolbar := container.NewHBox(
		addSelectedBtn,
		selectAllBtn,
		clearSelectionBtn,
		layout.NewSpacer(),
		clearClipBtn,
	)
	clipCardContent := container.NewBorder(
		clipToolbar,
		nil, nil, nil,
		container.NewHScroll(clipContainer),
	)
	clipTab := widget.NewCard(clipCardTitle, "", clipCardContent)

	// The Job Discovery Engine is temporarily disabled pending further testing.
	// searchTab is still built above so the feature can be re-enabled later; it
	// is intentionally not added to the visible sub-tabs for now.
	_ = searchTab

	subTabs := container.NewAppTabs(
		container.NewTabItem("Job Leads", clipTab),
	)

	return subTabs
}

func createKPICard(title string, valueLabel *widget.Label, borderClr color.Color) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{})
	rect := canvas.NewRectangle(borderClr)
	rect.SetMinSize(fyne.NewSize(150, 4))

	cardContent := container.NewVBox(
		titleLabel,
		valueLabel,
		rect,
	)

	background := canvas.NewRectangle(color.RGBA{R: 30, G: 41, B: 59, A: 120})

	return container.NewMax(
		background,
		container.NewBorder(nil, nil, nil, nil, cardContent),
	)
}

func updateDashboardStats() {
	var w, a, i, o, r int
	state.RecentBox.Objects = nil

	for idx, app := range state.Applications {
		switch app.Status {
		case "Wishlist":
			w++
		case "Applied":
			a++
		case "Interviewing":
			i++
		case "Offer":
			o++
		case "Rejected", "Ghosted":
			r++
		}

		if idx < 5 { // Display top 5 recent jobs
			dateLabel := widget.NewLabel(app.Date)
			jobText := fmt.Sprintf("%s - %s (%s)", app.Company, app.Role, app.Status)
			state.RecentBox.Add(container.NewHBox(
				widget.NewIcon(theme.FolderOpenIcon()),
				widget.NewLabel(jobText),
				layout.NewSpacer(),
				dateLabel,
			))
		}
	}

	state.WishlistLabel.SetText(fmt.Sprintf("%d", w))
	state.AppliedLabel.SetText(fmt.Sprintf("%d", a))
	state.InterviewLabel.SetText(fmt.Sprintf("%d", i))
	state.OfferLabel.SetText(fmt.Sprintf("%d", o))
	state.RejectedLabel.SetText(fmt.Sprintf("%d", r))

	state.RecentBox.Refresh()
}

// 2. BASE PROFILE VIEW
func buildProfileTab() fyne.CanvasObject {
	state.NameEntry = widget.NewEntry()
	state.EmailEntry = widget.NewEntry()
	state.PhoneEntry = widget.NewEntry()
	state.LocEntry = widget.NewEntry()
	state.LinkedinEntry = widget.NewEntry()
	state.WebsiteEntry = widget.NewEntry()
	state.RolesEntry = widget.NewEntry()
	state.TechSkillsEntry = widget.NewEntry()
	state.PmSkillsEntry = widget.NewEntry()

	state.EduContainer = container.NewVBox()
	state.ExpContainer = container.NewVBox()
	state.ProjContainer = container.NewVBox()
	state.AddlSectionsContainer = container.NewVBox()

	importBtn := widget.NewButtonWithIcon("Import PDF/DOCX Resume", theme.DocumentIcon(), func() {
		showCustomFilePicker(state.Window, "Import PDF/DOCX Resume", []string{".pdf", ".docx", ".txt", ".md"}, false, func(filePath string) {
			progress := dialog.NewProgressInfinite("Parsing Resume", "Reading and structuring using Gemini AI...", state.Window)
			progress.Show()

			go func() {
				outText, err := RunParseResume(filePath)
				if err != nil {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(err, state.Window)
					})
					return
				}

				if state.ApiKey == "" {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(fmt.Errorf("Gemini API Key is missing. Please set it in Settings first."), state.Window)
					})
					return
				}

				parsePrompt := fmt.Sprintf(`
You are an expert resume parsing AI. Extract the resume text below and convert it into a valid JSON object matching the following structure exactly. Do not add comments or additional text. Output ONLY valid JSON.

Structure:
{
  "personal_info": {
    "name": "Full Name",
    "email": "Email Address",
    "phone": "Phone Number",
    "location": "City, State",
    "linkedin": "linkedin URL (optional)",
    "website": "portfolio or personal website URL (optional)"
  },
  "target_roles": ["Role 1", "Role 2"],
  "education": [
    {
      "institution": "University Name",
      "degree": "Degree",
      "major": "Major",
      "graduation_date": "Date",
      "location": "City, State",
      "gpa": "",
      "details": ""
    }
  ],
  "experience": [
    {
      "company": "Company Name",
      "role": "Job Title",
      "location": "City, State",
      "start_date": "Month Year",
      "end_date": "Month Year or Present",
      "bullets": [
        "Detailed bullet point describing achievements and duties.",
        "Another bullet point."
      ]
    }
  ],
  "projects": [],
  "skills": {
    "technical": ["Python"],
    "product_management": []
  },
  "additional_sections": [
    { "title": "Section Title (e.g. Publications)", "items": ["entry 1", "entry 2"] }
  ]
}

Section detection rules (open to, but never presume, common résumé sections):
- Identify EVERY distinct section actually present in the résumé.
- Map standard sections to their typed fields (personal_info, target_roles, education, experience, projects, skills).
- For any other section the résumé actually contains — e.g. Publications, Research, Certifications, Licenses, Awards, Honors, Volunteer Experience, Languages, Patents, Speaking, Memberships — emit it under "additional_sections" as { "title", "items" }, preserving the author's original section title and wording.
- Do NOT invent sections that are not present. Do NOT force empty sections.
- If the résumé has none of these extra sections, output "additional_sections": [].

Resume Text:
%s`, outText)

				parsedJsonStr, err := callGeminiGo(state.ApiKey, parsePrompt, true)
				if err != nil {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(err, state.Window)
					})
					return
				}

				if err := writeSecureFile("references/user-profile.json", []byte(parsedJsonStr)); err != nil {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(err, state.Window)
					})
					return
				}

				// Reload the in-memory profile from the file we just wrote, then
				// repopulate the entire form on the UI thread. fillProfileForm MUST
				// run on the main goroutine: calling it here off-thread silently
				// dropped the personal-info Entry.SetText updates (name/email/phone/
				// location) while the section containers still re-rendered — which is
				// exactly why basic personal info never changed on a new résumé.
				loadProfileData()
				fyne.Do(func() {
					progress.Hide()
					fillProfileForm()
					refreshUI()
				})

				offerWorkspaceSetup(filePath)
			}()
		})
	})

	saveBtn := widget.NewButtonWithIcon("Save Profile Details", theme.DocumentSaveIcon(), func() {
		gatherProfileFromForm()
		saveProfileData()
		dialog.ShowInformation("Success", "Base Profile details successfully written to references/user-profile.json", state.Window)
		reloadAllViews()
	})

	fillProfileForm()

	contactForm := container.New(layout.NewFormLayout(),
		widget.NewLabel("Full Name"), state.NameEntry,
		widget.NewLabel("Email Address"), state.EmailEntry,
		widget.NewLabel("Phone Number"), state.PhoneEntry,
		widget.NewLabel("Location"), state.LocEntry,
		widget.NewLabel("LinkedIn Link"), state.LinkedinEntry,
		widget.NewLabel("Portfolio/Website"), state.WebsiteEntry,
		widget.NewLabel("Target Job Titles"), state.RolesEntry,
		widget.NewLabel("Technical Skills"), state.TechSkillsEntry,
		widget.NewLabel("Product/Other Skills"), state.PmSkillsEntry,
	)

	addEduBtn := widget.NewButton("Add Education", func() {
		state.Profile.Education = append(state.Profile.Education, Education{})
		renderEducationForm()
	})
	addExpBtn := widget.NewButton("Add Experience", func() {
		state.Profile.Experience = append(state.Profile.Experience, Experience{})
		renderExperienceForm()
	})
	addProjBtn := widget.NewButton("Add Project", func() {
		state.Profile.Projects = append(state.Profile.Projects, Project{})
		renderProjectForm()
	})

	profileContent := container.NewVBox(
		container.NewHBox(importBtn, saveBtn),
		widget.NewSeparator(),
		widget.NewCard("Personal Information", "", contactForm),
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Education Section", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), addEduBtn),
		state.EduContainer,
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Experience Section", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), addExpBtn),
		state.ExpContainer,
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Projects Section", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), addProjBtn),
		state.ProjContainer,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Additional Sections (auto-detected from your résumé)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		state.AddlSectionsContainer,
	)

	scroll := container.NewVScroll(profileContent)
	return container.NewStack(scroll, newScrollInterceptor(scroll))
}

func fillProfileForm() {
	if state.Profile == nil {
		return
	}
	state.NameEntry.SetText(state.Profile.PersonalInfo.Name)
	state.EmailEntry.SetText(state.Profile.PersonalInfo.Email)
	state.PhoneEntry.SetText(state.Profile.PersonalInfo.Phone)
	state.LocEntry.SetText(state.Profile.PersonalInfo.Location)
	state.LinkedinEntry.SetText(state.Profile.PersonalInfo.Linkedin)
	state.WebsiteEntry.SetText(state.Profile.PersonalInfo.Website)

	state.RolesEntry.SetText(strings.Join(state.Profile.TargetRoles, ", "))
	state.TechSkillsEntry.SetText(strings.Join(state.Profile.Skills["technical"], ", "))
	state.PmSkillsEntry.SetText(strings.Join(state.Profile.Skills["product_management"], ", "))

	renderEducationForm()
	renderExperienceForm()
	renderProjectForm()
	renderAdditionalSectionsForm()
}

func renderExperienceForm() {
	state.ExpContainer.Objects = nil
	for idx, exp := range state.Profile.Experience {
		compEnt := widget.NewEntry()
		compEnt.SetText(exp.Company)
		roleEnt := widget.NewEntry()
		roleEnt.SetText(exp.Role)
		locEnt := widget.NewEntry()
		locEnt.SetText(exp.Location)

		datesEnt := widget.NewEntry()
		dateVal := exp.StartDate
		if exp.EndDate != "" {
			dateVal += " - " + exp.EndDate
		}
		datesEnt.SetText(dateVal)

		bulletsEnt := widget.NewMultiLineEntry()
		bulletsEnt.SetText(strings.Join(exp.Bullets, "\n"))
		bulletsEnt.SetMinRowsVisible(4)

		itemIdx := idx
		delBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			state.Profile.Experience = append(state.Profile.Experience[:itemIdx], state.Profile.Experience[itemIdx+1:]...)
			renderExperienceForm()
		})

		expForm := container.New(layout.NewFormLayout(),
			widget.NewLabel("Company"), compEnt,
			widget.NewLabel("Role"), roleEnt,
			widget.NewLabel("Location"), locEnt,
			widget.NewLabel("Dates"), datesEnt,
			widget.NewLabel("Bullets"), bulletsEnt,
		)

		card := widget.NewCard(fmt.Sprintf("Experience #%d", idx+1), "", container.NewVBox(expForm, delBtn))
		state.ExpContainer.Add(card)
	}
	state.ExpContainer.Refresh()
}

func renderEducationForm() {
	state.EduContainer.Objects = nil
	for idx, edu := range state.Profile.Education {
		instEnt := widget.NewEntry()
		instEnt.SetText(edu.Institution)
		degEnt := widget.NewEntry()
		degEnt.SetText(edu.Degree)
		majEnt := widget.NewEntry()
		majEnt.SetText(edu.Major)
		dateEnt := widget.NewEntry()
		dateEnt.SetText(edu.GraduationDate)
		gpaEnt := widget.NewEntry()
		gpaEnt.SetText(edu.GPA)

		itemIdx := idx
		delBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			state.Profile.Education = append(state.Profile.Education[:itemIdx], state.Profile.Education[itemIdx+1:]...)
			renderEducationForm()
		})

		eduForm := container.New(layout.NewFormLayout(),
			widget.NewLabel("Institution"), instEnt,
			widget.NewLabel("Degree"), degEnt,
			widget.NewLabel("Major"), majEnt,
			widget.NewLabel("Graduation Date"), dateEnt,
			widget.NewLabel("GPA"), gpaEnt,
		)

		card := widget.NewCard(fmt.Sprintf("Education #%d", idx+1), "", container.NewVBox(eduForm, delBtn))
		state.EduContainer.Add(card)
	}
	state.EduContainer.Refresh()
}

func renderProjectForm() {
	state.ProjContainer.Objects = nil
	for idx, proj := range state.Profile.Projects {
		nameEnt := widget.NewEntry()
		nameEnt.SetText(proj.Name)
		techEnt := widget.NewEntry()
		techEnt.SetText(strings.Join(proj.Technologies, ", "))
		descEnt := widget.NewEntry()
		descEnt.SetText(proj.Description)
		detailsEnt := widget.NewEntry()
		detailsEnt.SetText(proj.Details)

		itemIdx := idx
		delBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			state.Profile.Projects = append(state.Profile.Projects[:itemIdx], state.Profile.Projects[itemIdx+1:]...)
			renderProjectForm()
		})

		projForm := container.New(layout.NewFormLayout(),
			widget.NewLabel("Project Name"), nameEnt,
			widget.NewLabel("Technologies"), techEnt,
			widget.NewLabel("Description"), descEnt,
			widget.NewLabel("Achievements"), detailsEnt,
		)

		card := widget.NewCard(fmt.Sprintf("Project #%d", idx+1), "", container.NewVBox(projForm, delBtn))
		state.ProjContainer.Add(card)
	}
	state.ProjContainer.Refresh()
}

// renderAdditionalSectionsForm shows intuited résumé sections (Publications,
// Certifications, Awards, etc.) as read-only cards so users can confirm what
// was parsed. These round-trip through the Profile struct so saves and tailoring
// preserve them automatically. If none were parsed, render a placeholder note.
func renderAdditionalSectionsForm() {
	state.AddlSectionsContainer.Objects = nil
	if state.Profile == nil || len(state.Profile.AdditionalSections) == 0 {
		hint := widget.NewLabel("None detected. Sections like Publications, Certifications, or Awards will appear here if your résumé contains them.")
		hint.Wrapping = fyne.TextWrapWord
		state.AddlSectionsContainer.Add(hint)
		state.AddlSectionsContainer.Refresh()
		return
	}
	for idx, section := range state.Profile.AdditionalSections {
		items := widget.NewLabel(strings.Join(section.Items, "\n"))
		items.Wrapping = fyne.TextWrapWord
		card := widget.NewCard(fmt.Sprintf("%s (#%d, %d items)", section.Title, idx+1, len(section.Items)), "preserved verbatim during tailoring", items)
		state.AddlSectionsContainer.Add(card)
	}
	state.AddlSectionsContainer.Refresh()
}

func gatherProfileFromForm() {
	if state.Profile == nil {
		return
	}
	state.Profile.PersonalInfo.Name = state.NameEntry.Text
	state.Profile.PersonalInfo.Email = state.EmailEntry.Text
	state.Profile.PersonalInfo.Phone = state.PhoneEntry.Text
	state.Profile.PersonalInfo.Location = state.LocEntry.Text
	state.Profile.PersonalInfo.Linkedin = state.LinkedinEntry.Text
	state.Profile.PersonalInfo.Website = state.WebsiteEntry.Text

	state.Profile.TargetRoles = splitCsv(state.RolesEntry.Text)
	state.Profile.Skills["technical"] = splitCsv(state.TechSkillsEntry.Text)
	state.Profile.Skills["product_management"] = splitCsv(state.PmSkillsEntry.Text)

	// Gather Experience
	for idx, child := range state.ExpContainer.Objects {
		card := child.(*widget.Card)
		cardContent := card.Content.(*fyne.Container)
		formLayout := cardContent.Objects[0].(*fyne.Container)

		comp := formLayout.Objects[1].(*widget.Entry).Text
		role := formLayout.Objects[3].(*widget.Entry).Text
		loc := formLayout.Objects[5].(*widget.Entry).Text
		dates := formLayout.Objects[7].(*widget.Entry).Text
		bullets := formLayout.Objects[9].(*widget.Entry).Text

		start, end := splitDates(dates)
		state.Profile.Experience[idx] = Experience{
			Company:   comp,
			Role:      role,
			Location:  loc,
			StartDate: start,
			EndDate:   end,
			Bullets:   strings.Split(bullets, "\n"),
		}
	}

	// Gather Education
	for idx, child := range state.EduContainer.Objects {
		card := child.(*widget.Card)
		cardContent := card.Content.(*fyne.Container)
		formLayout := cardContent.Objects[0].(*fyne.Container)

		inst := formLayout.Objects[1].(*widget.Entry).Text
		deg := formLayout.Objects[3].(*widget.Entry).Text
		maj := formLayout.Objects[5].(*widget.Entry).Text
		grad := formLayout.Objects[7].(*widget.Entry).Text
		gpa := formLayout.Objects[9].(*widget.Entry).Text

		state.Profile.Education[idx] = Education{
			Institution:    inst,
			Degree:         deg,
			Major:          maj,
			GraduationDate: grad,
			GPA:            gpa,
		}
	}

	// Gather Projects
	for idx, child := range state.ProjContainer.Objects {
		card := child.(*widget.Card)
		cardContent := card.Content.(*fyne.Container)
		formLayout := cardContent.Objects[0].(*fyne.Container)

		name := formLayout.Objects[1].(*widget.Entry).Text
		tech := formLayout.Objects[3].(*widget.Entry).Text
		desc := formLayout.Objects[5].(*widget.Entry).Text
		details := formLayout.Objects[7].(*widget.Entry).Text

		state.Profile.Projects[idx] = Project{
			Name:         name,
			Technologies: splitCsv(tech),
			Description:  desc,
			Details:      details,
		}
	}
}

func splitCsv(s string) []string {
	parts := strings.Split(s, ",")
	var list []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}

func splitDates(s string) (string, string) {
	parts := strings.Split(s, "-")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return s, ""
}

type clickableCell struct {
	widget.BaseWidget
	text   *canvas.Text
	cellID widget.TableCellID
}

func newClickableCell() *clickableCell {
	c := &clickableCell{
		text: canvas.NewText("", theme.ForegroundColor()),
	}
	c.text.TextSize = 12
	c.text.Alignment = fyne.TextAlignLeading
	c.ExtendBaseWidget(c)
	return c
}

type clickableCellRenderer struct {
	cell *clickableCell
}

func (r *clickableCellRenderer) Destroy() {}
func (r *clickableCellRenderer) Layout(size fyne.Size) {
	r.cell.text.Resize(fyne.NewSize(size.Width-8, size.Height-4))
	r.cell.text.Move(fyne.NewPos(4, 2))
}
func (r *clickableCellRenderer) MinSize() fyne.Size {
	return r.cell.text.MinSize()
}
func (r *clickableCellRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.cell.text}
}
func (r *clickableCellRenderer) Refresh() {
	r.cell.text.Color = theme.ForegroundColor()
	canvas.Refresh(r.cell.text)
}

func (c *clickableCell) CreateRenderer() fyne.WidgetRenderer {
	return &clickableCellRenderer{cell: c}
}

func (c *clickableCell) Tapped(ev *fyne.PointEvent) {
	state.TrackerTable.Select(c.cellID)
}

func (c *clickableCell) DoubleTapped(ev *fyne.PointEvent) {
	state.TrackerTable.Select(c.cellID)
	if c.cellID.Row > 0 && c.cellID.Row-1 < len(state.Applications) {
		app := &state.Applications[c.cellID.Row-1]
		openAddJobModal(app, c.cellID.Col)
	}
}

type tableTheme struct {
	fyne.Theme
}

func (t *tableTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameSeparator {
		return color.Transparent
	}
	return t.Theme.Color(name, variant)
}

type statusCell struct {
	widget.BaseWidget
	text      *canvas.Text
	arrow     *canvas.Text
	cellID    widget.TableCellID
	selected  string
	onChanged func(string)
}

func newStatusCell() *statusCell {
	sc := &statusCell{
		text:  canvas.NewText("", theme.ForegroundColor()),
		arrow: canvas.NewText("▼", theme.ForegroundColor()),
	}
	sc.text.TextSize = 12
	sc.text.Alignment = fyne.TextAlignLeading
	sc.arrow.TextSize = 8
	sc.arrow.Alignment = fyne.TextAlignCenter
	sc.ExtendBaseWidget(sc)
	return sc
}

type statusCellRenderer struct {
	cell *statusCell
}

func (r *statusCellRenderer) Destroy() {}
func (r *statusCellRenderer) Layout(size fyne.Size) {
	r.cell.text.Resize(fyne.NewSize(size.Width-20, size.Height-4))
	r.cell.text.Move(fyne.NewPos(4, 2))

	r.cell.arrow.Resize(fyne.NewSize(12, size.Height-4))
	r.cell.arrow.Move(fyne.NewPos(size.Width-16, 2))
}
func (r *statusCellRenderer) MinSize() fyne.Size {
	ts := r.cell.text.MinSize()
	return fyne.NewSize(ts.Width+20, ts.Height)
}
func (r *statusCellRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.cell.text, r.cell.arrow}
}
func (r *statusCellRenderer) Refresh() {
	r.cell.text.Color = theme.ForegroundColor()
	r.cell.arrow.Color = theme.ForegroundColor()
	canvas.Refresh(r.cell.text)
	canvas.Refresh(r.cell.arrow)
}

func (sc *statusCell) CreateRenderer() fyne.WidgetRenderer {
	return &statusCellRenderer{cell: sc}
}

func (sc *statusCell) Tapped(ev *fyne.PointEvent) {
	statuses := []string{"Wishlist", "Applied", "Interviewing", "Offer", "Rejected", "Ghosted"}
	var items []*fyne.MenuItem
	for _, status := range statuses {
		s := status
		items = append(items, fyne.NewMenuItem(s, func() {
			oldStatus := sc.selected
			if oldStatus == s {
				return // No change
			}

			sc.selected = s
			sc.text.Text = s
			sc.Refresh()

			if sc.cellID.Row > 0 && sc.cellID.Row-1 < len(state.Applications) {
				state.Applications[sc.cellID.Row-1].Status = s
			}

			// The cell already repainted itself above (sc.text + sc.Refresh), and
			// the disk write runs in the background via onChanged, so only the
			// dashboard counts need updating. Avoid the full refreshUI() here — its
			// whole-table Refresh was the source of the per-change sluggishness.
			updateDashboardStats()

			if sc.onChanged != nil {
				sc.onChanged(s)
			}
		}))
	}

	menu := fyne.NewMenu("", items...)
	popUp := widget.NewPopUpMenu(menu, state.Window.Canvas())
	popUp.ShowAtPosition(ev.AbsolutePosition)

	// Defer row selection to after the popup renders. Doing it before causes
	// trackerStatusSelect.SetSelected + table refresh to block popup display.
	cellID := sc.cellID
	go func() {
		fyne.Do(func() {
			state.TrackerTable.Select(cellID)
		})
	}()
}

func (sc *statusCell) DoubleTapped(ev *fyne.PointEvent) {
	state.TrackerTable.Select(sc.cellID)
	if sc.cellID.Row > 0 && sc.cellID.Row-1 < len(state.Applications) {
		app := &state.Applications[sc.cellID.Row-1]
		openAddJobModal(app, sc.cellID.Col)
	}
}

type trackerCell struct {
	widget.BaseWidget
	clickable *clickableCell
	status    *statusCell
	check     *widget.Check
	cellID    widget.TableCellID
}

func newTrackerCell() *trackerCell {
	tc := &trackerCell{
		clickable: newClickableCell(),
		status:    newStatusCell(),
		check:     widget.NewCheck("", nil),
	}
	tc.ExtendBaseWidget(tc)
	return tc
}

type trackerCellRenderer struct {
	cell *trackerCell
}

func (r *trackerCellRenderer) Destroy() {}
func (r *trackerCellRenderer) Layout(size fyne.Size) {
	r.cell.clickable.Resize(size)
	r.cell.clickable.Move(fyne.NewPos(0, 0))

	r.cell.status.Resize(size)
	r.cell.status.Move(fyne.NewPos(0, 0))

	r.cell.check.Resize(size)
	r.cell.check.Move(fyne.NewPos(0, 0))
}
func (r *trackerCellRenderer) MinSize() fyne.Size {
	return r.cell.clickable.MinSize()
}
func (r *trackerCellRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.cell.clickable, r.cell.status, r.cell.check}
}
func (r *trackerCellRenderer) Refresh() {
	r.cell.clickable.Refresh()
	r.cell.status.Refresh()
	r.cell.check.Refresh()
}

func (tc *trackerCell) CreateRenderer() fyne.WidgetRenderer {
	return &trackerCellRenderer{cell: tc}
}

// scrollInterceptor is a transparent, non-interactive widget placed on top of a
// scroll container via container.NewStack. It captures scroll events (and only
// scroll events — it implements no tap/drag interfaces so those fall through to
// content behind it) and multiplies the delta before forwarding, making long
// pages like the profile tab scroll at a comfortable speed.
type scrollInterceptor struct {
	widget.BaseWidget
	target *container.Scroll
}

func newScrollInterceptor(target *container.Scroll) *scrollInterceptor {
	si := &scrollInterceptor{target: target}
	si.ExtendBaseWidget(si)
	return si
}

type emptyRenderer struct{}

func (e *emptyRenderer) Destroy()                     {}
func (e *emptyRenderer) Layout(_ fyne.Size)           {}
func (e *emptyRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (e *emptyRenderer) Objects() []fyne.CanvasObject { return nil }
func (e *emptyRenderer) Refresh()                     {}

func (si *scrollInterceptor) CreateRenderer() fyne.WidgetRenderer {
	return &emptyRenderer{}
}

func (si *scrollInterceptor) Scrolled(ev *fyne.ScrollEvent) {
	boosted := *ev
	boosted.Scrolled.DY *= 3
	si.target.Scrolled(&boosted)
}

func (si *scrollInterceptor) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

type clipperRowLayout struct{}

// clipperCheckWidth is the fixed width of the leading selection-checkbox column.
const clipperCheckWidth = float32(40)

// clipperActionsWidth is the fixed width reserved for the row's action button.
// Only "View" remains (bulk add/tailor moved to the toolbar and the Tracker), so
// this is far narrower than before. Applied uniformly to header and data rows so
// their columns stay aligned.
const clipperActionsWidth = float32(120)

// clipperRowMinWidth is the minimum overall row width: the checkbox + actions
// columns plus a 140px floor for each of the four data columns.
const clipperRowMinWidth = clipperCheckWidth + clipperActionsWidth + 4*140

func (l *clipperRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(clipperRowMinWidth, 32)
}

// Layout positions six children: [checkbox] [company] [role] [location] [source]
// [actions]. The checkbox and actions columns are fixed width; the four data
// columns share the remainder equally.
func (l *clipperRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 6 {
		return
	}
	w := size.Width
	if w < clipperRowMinWidth {
		w = clipperRowMinWidth
	}
	colWidth := (w - clipperCheckWidth - clipperActionsWidth) / 4

	x := float32(0)
	objects[0].Move(fyne.NewPos(x, 0))
	objects[0].Resize(fyne.NewSize(clipperCheckWidth, size.Height))
	x += clipperCheckWidth
	for i := 1; i <= 4; i++ {
		objects[i].Move(fyne.NewPos(x, 0))
		objects[i].Resize(fyne.NewSize(colWidth, size.Height))
		x += colWidth
	}
	objects[5].Move(fyne.NewPos(x, 0))
	objects[5].Resize(fyne.NewSize(clipperActionsWidth, size.Height))
}

type fixedHeightLayout struct {
	height float32
}

func (l *fixedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	minW := clipperRowMinWidth
	for _, o := range objects {
		if min := o.MinSize(); min.Width > minW {
			minW = min.Width
		}
	}
	return fyne.NewSize(minW, l.height)
}

func (l *fixedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	w := size.Width
	if w < clipperRowMinWidth {
		w = clipperRowMinWidth
	}
	for _, o := range objects {
		o.Resize(fyne.NewSize(w, l.height))
		o.Move(fyne.NewPos(0, 0))
	}
}

type customTableLayout struct{}

func (l *customTableLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(600, 300)
}

func (l *customTableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Columns 0-5 total 920px. Give Notes the remaining space minus the 6
	// inter-column separator dividers so total content width == widget width.
	notesWidth := size.Width - 920 - float32(6)*theme.SeparatorThicknessSize()
	if notesWidth < 1 {
		notesWidth = 1
	}
	if state.TrackerTable != nil {
		state.TrackerTable.SetColumnWidth(6, notesWidth)
	}
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

var (
	trackerStatusSelect       *widget.Select
	trackerOpenResumeBtn      *widget.Button
	trackerOpenCoverLetterBtn *widget.Button
	trackerOpenLinkBtn        *widget.Button
	isUpdatingTrackerDropdown bool
)

// 3. JOB TRACKER SPREADSHEET TABLE VIEW
func buildTrackerTab() fyne.CanvasObject {
	// Status select dropdown on toolbar
	trackerStatusSelect = widget.NewSelect([]string{"Wishlist", "Applied", "Interviewing", "Offer", "Rejected", "Ghosted"}, func(selected string) {
		if isUpdatingTrackerDropdown {
			return
		}
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 || selected == "" {
			return
		}

		// Update the in-memory model immediately for instant UI feedback. The
		// previous implementation spawned an external Python process per row and
		// then reloaded every view, which made each status change feel sluggish.
		changed := false
		for _, idx := range indices {
			if idx < len(state.Applications) && state.Applications[idx].Status != selected {
				state.Applications[idx].Status = selected
				changed = true
			}
		}
		if !changed {
			return
		}

		if state.TrackerTable != nil {
			state.TrackerTable.Refresh()
		}
		updateDashboardStats()

		// Persist to disk in the background so the dropdown stays responsive.
		go func() {
			if err := saveTrackerDataGo(); err != nil {
				fyne.Do(func() { dialog.ShowError(err, state.Window) })
			}
		}()
	})
	trackerStatusSelect.PlaceHolder = "Change Status"

	// Document opening buttons on toolbar
	trackerOpenResumeBtn = widget.NewButtonWithIcon("Open Resume", theme.DocumentIcon(), func() {
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 {
			return
		}
		openDocument(state.Applications[indices[0]].Resume, "resume")
	})
	trackerOpenResumeBtn.Disable()

	// Document opening buttons on toolbar
	trackerOpenCoverLetterBtn = widget.NewButtonWithIcon("Open Cover Letter", theme.DocumentIcon(), func() {
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 {
			return
		}
		openDocument(state.Applications[indices[0]].CoverLetter, "cover letter")
	})
	trackerOpenCoverLetterBtn.Disable()

	// Opens the job posting URL of the first selected row in the browser.
	trackerOpenLinkBtn = widget.NewButtonWithIcon("Open Job URL", theme.SearchIcon(), func() {
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 {
			return
		}
		// Collect valid, deduped http/file links from every selected row.
		seen := make(map[string]bool)
		var links []string
		for _, idx := range indices {
			if idx < 0 || idx >= len(state.Applications) {
				continue
			}
			link := strings.TrimSpace(state.Applications[idx].Link)
			lower := strings.ToLower(link)
			if link == "" || (!strings.HasPrefix(lower, "http") && !strings.HasPrefix(lower, "file")) {
				continue
			}
			if seen[link] {
				continue
			}
			seen[link] = true
			links = append(links, link)
		}
		if len(links) == 0 {
			return
		}
		open := func() {
			for _, link := range links {
				openLink(link)
			}
		}
		// Confirm before opening many tabs at once — guards against an
		// accidental "select all" sending dozens of pages to the browser.
		const confirmThreshold = 10
		if len(links) > confirmThreshold {
			msg := fmt.Sprintf("Open %d job URLs in your browser?", len(links))
			dialog.ShowConfirm("Open Multiple URLs", msg, func(ok bool) {
				if ok {
					open()
				}
			}, state.Window)
			return
		}
		open()
	})
	trackerOpenLinkBtn.Disable()

	// Table widget setup for spreadsheet-like grid layout
	state.TrackerTable = widget.NewTable(
		func() (int, int) {
			return len(state.Applications) + 1, 7 // 7 columns
		},
		func() fyne.CanvasObject {
			return newTrackerCell()
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			tc := cell.(*trackerCell)
			tc.cellID = id
			tc.clickable.cellID = id
			tc.status.cellID = id

			if id.Row == 0 {
				tc.clickable.Show()
				tc.status.Hide()
				tc.check.Hide()

				t := tc.clickable.text
				headers := []string{"Select", "Company", "Role", "Location", "Date", "Status", "Notes"}
				t.Text = headers[id.Col]
				t.TextStyle = fyne.TextStyle{Bold: true}
				t.TextSize = 13 // slightly larger for headers
				tc.clickable.Refresh()
			} else {
				if id.Row-1 < len(state.Applications) {
					app := state.Applications[id.Row-1]

					if id.Col == 0 {
						tc.clickable.Hide()
						tc.status.Hide()
						tc.check.Show()

						rowIdx := id.Row - 1
						tc.check.OnChanged = nil
						tc.check.SetChecked(trackerSelectedRows[rowIdx])
						tc.check.OnChanged = func(checked bool) {
							trackerSelectedRows[rowIdx] = checked
							updateTrackerSelectionUI()
						}
						tc.check.Refresh()
					} else if id.Col == 5 {
						tc.clickable.Hide()
						tc.status.Show()
						tc.check.Hide()

						tc.status.selected = app.Status
						tc.status.text.Text = app.Status

						// statusCell.Tapped already updates state.Applications in
						// memory and calls refreshUI(), so onChanged only needs to
						// persist to disk. Doing this in the background keeps the
						// dropdown responsive (the old path re-read the file and
						// rebuilt every view on each change).
						tc.status.onChanged = func(selected string) {
							if selected != "" {
								go func() {
									if err := saveTrackerDataGo(); err != nil {
										fyne.Do(func() { dialog.ShowError(err, state.Window) })
									}
								}()
							}
						}
						tc.status.Refresh()
					} else {
						tc.clickable.Show()
						tc.status.Hide()
						tc.check.Hide()

						t := tc.clickable.text
						t.TextStyle = fyne.TextStyle{}
						t.TextSize = 12 // compact size for data cells

						switch id.Col {
						case 1:
							t.Text = app.Company
						case 2:
							t.Text = app.Role
						case 3:
							t.Text = app.Location
						case 4:
							t.Text = app.Date
						case 6:
							t.Text = app.Notes
						}
						tc.clickable.Refresh()
					}
				}
			}
			tc.Refresh()
		},
	)

	// Set column widths to look like spreadsheet grid
	state.TrackerTable.SetColumnWidth(0, trackerColSelectWidth) // Select
	state.TrackerTable.SetColumnWidth(1, 240)                   // Company
	state.TrackerTable.SetColumnWidth(2, 280)                   // Role
	state.TrackerTable.SetColumnWidth(3, 110)                   // Location
	state.TrackerTable.SetColumnWidth(4, 90)                    // Date
	state.TrackerTable.SetColumnWidth(5, 140)                   // Status
	// Notes (col 6) width is managed entirely by customTableLayout.Layout

	state.TrackerTable.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row-1 < len(state.Applications) {
			state.SelectedAppIdx = id.Row - 1
			updateTrackerSelectionUI()
		}
	}

	addBtn := widget.NewButtonWithIcon("Add Application", theme.ContentAddIcon(), func() {
		openAddJobModal(nil, -1)
	})

	updateBtn := widget.NewButtonWithIcon("Update Details", theme.DocumentCreateIcon(), func() {
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 {
			dialog.ShowInformation("Selection Required", "Please select or check a job in the table first.", state.Window)
			return
		}
		if len(indices) > 1 {
			dialog.ShowInformation("Single Selection Required", "Please check or select only one job to update.", state.Window)
			return
		}
		openAddJobModal(&state.Applications[indices[0]], -1)
	})

	deleteBtn := widget.NewButtonWithIcon("Delete Application", theme.DeleteIcon(), func() {
		indices := getSelectedTrackerIndices()
		if len(indices) == 0 {
			dialog.ShowInformation("Selection Required", "Please select or check a job in the table first.", state.Window)
			return
		}

		msg := "Are you sure you want to delete the selected job application?"
		if len(indices) > 1 {
			msg = fmt.Sprintf("Are you sure you want to delete the %d selected job applications?", len(indices))
		}

		dialog.ShowConfirm("Confirm Delete", msg, func(confirmed bool) {
			if !confirmed {
				return
			}

			progress := dialog.NewProgressInfinite("Deleting Applications", "Removing job applications from tracker...", state.Window)
			progress.Show()

			go func() {
				var lastErr error
				for _, idx := range indices {
					app := &state.Applications[idx]
					err := deleteApplicationGo(app.Company, app.Role)
					if err != nil {
						lastErr = err
					}
				}
				fyne.Do(func() {
					progress.Hide()
					if lastErr != nil {
						dialog.ShowError(lastErr, state.Window)
					}
					// Clear checkbox selections to prevent index out of bounds on reload
					for k := range trackerSelectedRows {
						delete(trackerSelectedRows, k)
					}
					reloadAllViews()
				})
			}()
		}, state.Window)
	})

	// The "Sync Email Updates" (IMAP) button is temporarily disabled while the
	// email-sync feature is hidden. RunManageApplications("sync", ...) remains
	// available so the button can be restored in the future.

	// Relocated Bulk Import Button
	bulkImportBtn := widget.NewButtonWithIcon("Bulk Import", theme.FolderOpenIcon(), func() {
		showCustomFilePicker(state.Window, "Select CSV File", []string{".csv", ".txt"}, false, func(path string) {
			go func() {
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					fyne.Do(func() { dialog.ShowError(readErr, state.Window) })
					return
				}
				lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
				added := 0
				skipped := 0
				for i, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					// Skip header row if it doesn't look like a URL
					if i == 0 && !strings.HasPrefix(strings.ToLower(line), "http") {
						continue
					}
					// Parse CSV fields: URL, Company, Role, Location (all optional after URL)
					fields := strings.SplitN(line, ",", 5)
					jobURL := strings.Trim(strings.TrimSpace(fields[0]), "\"")
					if !strings.HasPrefix(strings.ToLower(jobURL), "http") {
						skipped++
						continue
					}
					company := ""
					if len(fields) > 1 {
						company = strings.Trim(strings.TrimSpace(fields[1]), "\"")
					}
					role := ""
					if len(fields) > 2 {
						role = strings.Trim(strings.TrimSpace(fields[2]), "\"")
					}
					location := ""
					if len(fields) > 3 {
						location = strings.Trim(strings.TrimSpace(fields[3]), "\"")
					}
					// Derive company from domain if not provided
					if company == "" {
						company = sourceBadge(jobURL)
					}
					if role == "" {
						role = "(Imported — update role)"
					}
					resumeName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(company, " ", "_"))
					coverName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(company, " ", "_"))
					addErr := addApplicationGo(company, role, location, jobURL, "Applied", resumeName, coverName, "Bulk imported from CSV.")
					if addErr == nil {
						added++
					} else {
						skipped++
					}
				}
				fyne.Do(func() {
					reloadAllViews()
					dialog.ShowInformation("Import Complete",
						fmt.Sprintf("%d job(s) imported, %d skipped.\n\nOpen the Tracker tab to update roles and details.", added, skipped),
						state.Window)
				})
			}()
		})
	})

	// Forward-declared so the button's mode-selection callback can invoke it;
	// the implementation is assigned just below.
	var runTailorSelected func(selectedIndices []int, doResume, doCover bool)

	tailorSelectedBtn := widget.NewButtonWithIcon("Tailor Selected", theme.SettingsIcon(), func() {
		var selectedIndices []int
		for rowIdx, selected := range trackerSelectedRows {
			if selected && rowIdx < len(state.Applications) {
				selectedIndices = append(selectedIndices, rowIdx)
			}
		}

		if len(selectedIndices) == 0 {
			dialog.ShowInformation("Selection Required", "Please select/check at least one application from the table.", state.Window)
			return
		}

		modeRadio := widget.NewRadioGroup(getTailorModeOptions(), nil)
		modeRadio.SetSelected("Both")
		dialog.ShowCustomConfirm("Tailor Selected", "Tailor", "Cancel",
			container.NewVBox(
				widget.NewLabel("What would you like to tailor for the selected application(s)?"),
				modeRadio,
			),
			func(confirmed bool) {
				if !confirmed {
					return
				}
				mode := modeRadio.Selected
				if mode == "" {
					return
				}
				doResume := mode == "Resume" || mode == "Both"
				doCover := mode == "Cover Letter" || mode == "Both"

				runTailorSelected(selectedIndices, doResume, doCover)
			}, state.Window)
	})
	if !resumeTailoringEnabled {
		tailorSelectedBtn.Hide()
	}

	// When resume tailoring is disabled, expose a focused "Generate Cover
	// Letters" button so users can still bulk-draft cover letters for selected
	// applications. Internally this reuses runTailorSelected with doResume=false.
	bulkCoverLettersBtn := widget.NewButtonWithIcon("Generate Cover Letters", theme.MailComposeIcon(), func() {
		var selectedIndices []int
		for rowIdx, selected := range trackerSelectedRows {
			if selected && rowIdx < len(state.Applications) {
				selectedIndices = append(selectedIndices, rowIdx)
			}
		}
		if len(selectedIndices) == 0 {
			dialog.ShowInformation("Selection Required", "Please select/check at least one application from the table.", state.Window)
			return
		}
		dialog.ShowConfirm("Generate Cover Letters",
			fmt.Sprintf("Draft and compile cover letter PDFs for %d selected application(s)?", len(selectedIndices)),
			func(ok bool) {
				if ok {
					runTailorSelected(selectedIndices, false, true)
				}
			}, state.Window)
	})
	if resumeTailoringEnabled {
		bulkCoverLettersBtn.Hide()
	}

	runTailorSelected = func(selectedIndices []int, doResume, doCover bool) {
		progress := dialog.NewProgressInfinite("Bulk Tailoring Assets", fmt.Sprintf("Processing %d selected applications sequentially...", len(selectedIndices)), state.Window)
		progress.Show()

		go func() {
			baseProfileBytes, err := os.ReadFile("references/user-profile.json")
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}

			var prof Profile
			json.Unmarshal(baseProfileBytes, &prof)
			candName := prof.PersonalInfo.Name
			if candName == "" {
				candName = "[Full Name]"
			}

			var errorsList []string
			for _, idx := range selectedIndices {
				app := state.Applications[idx]

				comp := titleSanitizerRegex.ReplaceAllString(app.Company, "")
				comp = strings.TrimSpace(comp)
				role := titleSanitizerRegex.ReplaceAllString(app.Role, "")
				role = strings.TrimSpace(role)

				resumePdfName := app.Resume
				if resumePdfName == "" {
					resumePdfName = fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(comp, " ", "_"))
				}
				coverLetterPdfName := app.CoverLetter
				if coverLetterPdfName == "" {
					coverLetterPdfName = fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(comp, " ", "_"))
				}

				desc := findClippedJobDescription(app.Company, app.Role, app.Link)
				if desc == "" {
					desc = app.Notes
					if strings.HasPrefix(desc, "Clipped from browser. ") {
						desc = strings.TrimPrefix(desc, "Clipped from browser. ")
					}
				}
				if strings.TrimSpace(desc) == "" {
					desc = fmt.Sprintf("Role: %s at %s", role, comp)
				}

				// 1. Tailor Resume
				if doResume {
					tailorPrompt := fmt.Sprintf(`You are an expert resume writer. Rewrite ONLY the applicant's experience bullet points in the base profile JSON to align with the target job description. All other fields must be preserved verbatim.

Base Profile JSON:
%s

Target Job Description:
%s

Mandates:
1. Rewrite ONLY the experience bullet points. Use the STAR framework (Situation, Task, Action, Result) and begin each bullet with a strong active past-tense verb.
2. Naturally incorporate relevant keywords from the target job description into the rewritten bullets.
3. STRICTLY preserve every quantitative metric (percentages, dollar amounts, team sizes, time periods, dates). Never fabricate, omit, or alter any number.
4. Preserve VERBATIM: personal_info, target_roles, education, projects, skills, and every entry under additional_sections (publications, certifications, awards, etc.). Do not add, remove, reorder, or rename any field. Do not invent skills the applicant did not list. Do not create new sections.
5. Output ONLY valid JSON with the EXACT same set of top-level and nested keys as the input. No explanations, no markdown blocks.`, string(baseProfileBytes), desc)

					tailoredJson, err := callGeminiGo(state.ApiKey, tailorPrompt, true)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: resume tailoring failed: %v", comp, role, err))
						continue
					}

					tailoredBytes := []byte(tailoredJson)
					if verr := validateTailoredProfile(baseProfileBytes, tailoredBytes); verr != nil {
						fmt.Printf("Tailored profile guard tripped for %s @ %s (%v) — falling back to base profile.\n", role, comp, verr)
						tailoredBytes = baseProfileBytes
					}
					err = writeSecureFile("references/user-profile-tailored.json", tailoredBytes)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: failed to write tailored JSON: %v", comp, role, err))
						continue
					}

					resumeOutputPath := filepath.Join(state.SaveFolder, resumePdfName)
					_, err = RunGenerateResume("references/user-profile-tailored.json", resumeOutputPath)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: resume compilation failed: %v", comp, role, err))
						continue
					}
				}

				// 2. Draft Cover Letter
				if doCover {
					genericTemplate, genErr := ensureGenericCoverLetter(state.ApiKey, baseProfileBytes)
					if genErr != nil {
						fmt.Println("Warning: Could not load or generate generic cover letter template:", genErr)
					}

					clRole := stripRoleMetadata(role)
					coverPrompt := fmt.Sprintf(`Write a professional 4-paragraph cover letter for %s for the role of "%s" at "%s".

Important: Base the tone, style, and structure on the following generic template:
---
%s
---

Structure the cover letter exactly as follows:
Paragraph 1: State the position applied for ("%s" at "%s") and a hook showing alignment with company mission based on the provided job description:
"%s"
Paragraph 2-3: Map 2 specific, quantified achievements from the provided profile to the target requirements.
Paragraph 4: Professional closing and call to action.

Use the following profile details: %s

Strict Mandates:
1. Do NOT mention where the job listing was found or reference referral sources (such as LinkedIn, Indeed, etc.).
2. At the end of the letter, output only the sign-off 'Sincerely,' followed by the applicant's name. Do NOT output any other details (such as address, phone number, email, date, etc.) below the name or signature.

Output ONLY the cover letter text, no conversational intro or outro.`, candName, clRole, comp, genericTemplate, clRole, comp, desc, string(baseProfileBytes))

					coverLetterDraftText, err := callGeminiGo(state.ApiKey, coverPrompt, false)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: cover letter draft failed: %v", comp, role, err))
						continue
					}

					tempDraftPath := filepath.Join(os.TempDir(), fmt.Sprintf("legaj_bulk_draft_%d.txt", time.Now().UnixNano()))
					err = os.WriteFile(tempDraftPath, []byte(coverLetterDraftText), 0644)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: failed to write temp cover letter draft: %v", comp, role, err))
						continue
					}

					coverOutputPath := filepath.Join(state.SaveFolder, coverLetterPdfName)
					_, err = RunGenerateCoverLetter("references/user-profile.json", tempDraftPath, coverOutputPath)
					os.Remove(tempDraftPath)
					if err != nil {
						errorsList = append(errorsList, fmt.Sprintf("%s - %s: cover letter compilation failed: %v", comp, role, err))
						continue
					}
				}
			}

			var tailoredLabel string
			switch {
			case doResume && doCover:
				tailoredLabel = "resume & cover letter PDFs"
			case doResume:
				tailoredLabel = "resume PDFs"
			default:
				tailoredLabel = "cover letter PDFs"
			}

			fyne.Do(func() {
				progress.Hide()
				for k := range trackerSelectedRows {
					trackerSelectedRows[k] = false
				}
				reloadAllViews()
				if len(errorsList) > 0 {
					dialog.ShowError(fmt.Errorf("Bulk tailoring finished with errors:\n%s", strings.Join(errorsList, "\n")), state.Window)
				} else {
					dialog.ShowInformation("Bulk Tailoring Complete", fmt.Sprintf("Successfully tailored %s for %d application(s)!", tailoredLabel, len(selectedIndices)), state.Window)
				}
			})
		}()
	}

	controlBar := container.NewHBox(
		addBtn,
		bulkImportBtn,
		updateBtn,
		deleteBtn,
		tailorSelectedBtn,
		bulkCoverLettersBtn,
		widget.NewSeparator(),
		trackerOpenResumeBtn,
		trackerOpenCoverLetterBtn,
		trackerOpenLinkBtn,
		layout.NewSpacer(),
	)

	tableContainer := container.New(&customTableLayout{}, state.TrackerTable)
	tableCard := widget.NewCard("Job Tracker", "", tableContainer)
	cardContainer := container.NewBorder(nil, nil, nil, nil, tableCard)

	return container.NewBorder(controlBar, nil, nil, nil, cardContainer)
}

func updateTrackerList() {
	if state.TrackerTable != nil {
		state.TrackerTable.Refresh()
	}
}

// openAddJobModal opens the Job Card editor. focusColumn is the table column the
// user double-clicked (1=Company, 2=Role, 3=Location, 6=Notes); pass -1 to open
// without focusing a specific field.
func openAddJobModal(job *JobApplication, focusColumn int) {
	compEnt := widget.NewEntry()
	roleEnt := widget.NewEntry()
	locEnt := widget.NewEntry()
	dateEnt := widget.NewEntry()
	linkEnt := widget.NewEntry()
	notesEnt := widget.NewEntry()

	statusSelect := widget.NewSelect([]string{"Wishlist", "Applied", "Interviewing", "Offer", "Rejected", "Ghosted"}, nil)
	statusSelect.SetSelected("Applied")

	dateEnt.SetText(time.Now().Format("2006-01-02"))

	if job != nil {
		compEnt.SetText(job.Company)
		roleEnt.SetText(job.Role)
		locEnt.SetText(job.Location)
		dateEnt.SetText(job.Date)
		linkEnt.SetText(job.Link)
		statusSelect.SetSelected(job.Status)
		notesEnt.SetText(job.Notes)
	}

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Company"), compEnt,
		widget.NewLabel("Role"), roleEnt,
		widget.NewLabel("Location"), locEnt,
		widget.NewLabel("Date"), dateEnt,
		widget.NewLabel("URL"), linkEnt,
		widget.NewLabel("Status"), statusSelect,
		widget.NewLabel("Notes"), notesEnt,
	)

	d := dialog.NewCustomConfirm("Job Card Details", "Save Changes", "Cancel", form, func(confirmed bool) {
		if !confirmed {
			return
		}

		if compEnt.Text == "" || roleEnt.Text == "" {
			dialog.ShowInformation("Validation", "Company name and job title are required fields.", state.Window)
			return
		}

		progress := dialog.NewProgressInfinite("Saving to Tracker", "Writing row to Excel sheet...", state.Window)
		progress.Show()

		go func() {
			var err error
			if job != nil {
				// Update all fields of the selected application in-place
				for i := range state.Applications {
					if state.Applications[i].Company == job.Company && state.Applications[i].Role == job.Role {
						state.Applications[i].Company = cleanText(compEnt.Text)
						state.Applications[i].Role = cleanText(roleEnt.Text)
						state.Applications[i].Location = cleanText(locEnt.Text)
						state.Applications[i].Date = cleanText(dateEnt.Text)
						state.Applications[i].Link = cleanText(linkEnt.Text)
						state.Applications[i].Status = statusSelect.Selected
						state.Applications[i].Notes = cleanText(notesEnt.Text)
						break
					}
				}
				err = saveTrackerDataGo()
			} else {
				resPdfName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(compEnt.Text, " ", "_"))
				clPdfName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(compEnt.Text, " ", "_"))
				err = addApplicationGo(compEnt.Text, roleEnt.Text, locEnt.Text, linkEnt.Text, statusSelect.Selected, resPdfName, clPdfName, notesEnt.Text)
			}

			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}
				reloadAllViews()
			})
		}()
	}, state.Window)
	d.Resize(fyne.NewSize(500, 450))
	d.Show()

	// Place the cursor in the field matching the double-clicked column.
	var focusTarget fyne.Focusable
	switch focusColumn {
	case 1:
		focusTarget = compEnt
	case 2:
		focusTarget = roleEnt
	case 3:
		focusTarget = locEnt
	case 6:
		focusTarget = notesEnt
	}
	if focusTarget != nil {
		state.Window.Canvas().Focus(focusTarget)
	}
}

// 4. DOCUMENT TAILORING VIEW
func buildTailoringTab() fyne.CanvasObject {
	state.TailorJobSelect = widget.NewSelect([]string{}, func(selected string) {
		// Prefill requirements if matching job is found
	})
	updateDropdownSelectors()

	state.TailorReqsEntry = widget.NewMultiLineEntry()
	state.TailorReqsEntry.SetPlaceHolder("Paste job description requirements and responsibilities here...")
	state.TailorReqsEntry.SetMinRowsVisible(6)

	state.OriginalPreview = widget.NewLabel("Original experience bullet points will appear here.")
	state.TailoredPreview = widget.NewLabel("Tailored experience bullet points will appear here.")
	state.OriginalPreview.Wrapping = fyne.TextWrapWord
	state.TailoredPreview.Wrapping = fyne.TextWrapWord

	state.TailorCompare = container.NewGridWithColumns(2,
		widget.NewCard("Base Resume Experience Bullets", "", container.NewScroll(state.OriginalPreview)),
		widget.NewCard("Tailored/Rewritten Experience Bullets", "", container.NewScroll(state.TailoredPreview)),
	)
	state.TailorCompare.Hide()

	tailorBtn := widget.NewButtonWithIcon("Tailor Resume Bullets", theme.SettingsIcon(), func() {
		if state.TailorJobSelect.Selected == "" {
			dialog.ShowInformation("Validation", "Please select a target job application.", state.Window)
			return
		}
		if state.TailorReqsEntry.Text == "" {
			dialog.ShowInformation("Validation", "Please paste the target job requirements.", state.Window)
			return
		}

		progress := dialog.NewProgressInfinite("Tailoring Resume", "Comparing profile details & rewriting bullets...", state.Window)
		progress.Show()

		tailorProfileAsync(state.TailorReqsEntry.Text, func(diffText string, err error) {
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}
			state.OriginalPreview.SetText("See references/user-profile.json for original experiences.")
			state.TailoredPreview.SetText(diffText)
			state.TailorCompare.Show()
			state.TailorCompare.Refresh()
		})
	})

	compileResumeBtn := widget.NewButtonWithIcon("Compile Tailored PDF", theme.DocumentIcon(), func() {
		progress := dialog.NewProgressInfinite("Compiling PDF", "Generating publication-quality PDF via ReportLab...", state.Window)
		progress.Show()

		go func() {
			outputPath := filepath.Join(state.SaveFolder, "Resume_Generated_Tailored.pdf")
			outText, err := RunGenerateResume("references/user-profile-tailored.json", outputPath)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}
				dialog.ShowInformation("PDF Generated", fmt.Sprintf("%s\nSaved to: %s", outText, outputPath), state.Window)
			})
		}()
	})

	compileCoverBtn := widget.NewButtonWithIcon("Compile Cover Letter PDF", theme.DocumentIcon(), func() {
		if state.TailorJobSelect.Selected == "" {
			dialog.ShowInformation("Validation", "Please select a target job application.", state.Window)
			return
		}

		parts := strings.SplitN(state.TailorJobSelect.Selected, " - ", 2)
		comp := parts[0]
		role := ""
		if len(parts) == 2 {
			role = parts[1]
		}

		progress := dialog.NewProgressInfinite("Compiling Cover Letter", "Drafting text and formatting PDF layout...", state.Window)
		progress.Show()

		go func() {
			outputPath := filepath.Join(state.SaveFolder, fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(comp, " ", "_")))

			profileBytes, _ := os.ReadFile("references/user-profile.json")
			var prof Profile
			json.Unmarshal(profileBytes, &prof)
			candName := prof.PersonalInfo.Name
			if candName == "" {
				candName = "[Full Name]"
			}

			genericTemplate, genErr := ensureGenericCoverLetter(state.ApiKey, profileBytes)
			if genErr != nil {
				fmt.Println("Warning: Could not load or generate generic cover letter template:", genErr)
			}

			draftPrompt := fmt.Sprintf(`Write a professional 4-paragraph cover letter for %s for the role of "%s" at "%s" based on the provided profile details: "%s".

Important: Base the tone, style, and structure on the following generic template:
---
%s
---

Structure the cover letter exactly as follows:
Paragraph 1: State the position applied for and a hook showing alignment with company mission.
Paragraph 2-3: Map 2 specific, quantified achievements from experience to the target role.
Paragraph 4: Professional closing and call to action.

Strict Mandates:
1. Do NOT mention where the job listing was found or reference referral sources (such as LinkedIn, Indeed, etc.).
2. At the end of the letter, output only the sign-off 'Sincerely,' followed by the applicant's name. Do NOT output any other details (such as address, phone number, email, date, etc.) below the name or signature.

Output ONLY the cover letter text, no conversational intro or outro.`, candName, role, comp, string(profileBytes), genericTemplate)

			coverLetterDraftText, err := callGeminiGo(state.ApiKey, draftPrompt, false)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}

			tempDraftPath := filepath.Join(os.TempDir(), fmt.Sprintf("legaj_manual_draft_%d.txt", time.Now().UnixNano()))
			os.WriteFile(tempDraftPath, []byte(coverLetterDraftText), 0644)
			defer os.Remove(tempDraftPath)

			outText, err := RunGenerateCoverLetter("references/user-profile.json", tempDraftPath, outputPath)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}
				dialog.ShowInformation("Cover Letter Generated", fmt.Sprintf("%s\nSaved to: %s", outText, outputPath), state.Window)
			})
		}()
	})

	actionRow := container.NewHBox(tailorBtn, compileResumeBtn, compileCoverBtn)

	content := container.NewVBox(
		canvas.NewText("Document Tailoring & Compiler", theme.PrimaryColor()),
		widget.NewLabel("Match experience bullets to target roles and render print-ready single-page PDFs."),
		container.New(layout.NewFormLayout(), widget.NewLabel("Target Job"), state.TailorJobSelect),
		state.TailorReqsEntry,
		actionRow,
		state.TailorCompare,
	)

	return container.NewScroll(content)
}

func updateDropdownSelectors() {
	var list []string
	for _, app := range state.Applications {
		list = append(list, fmt.Sprintf("%s - %s", app.Company, app.Role))
	}
	if state.TailorJobSelect != nil {
		state.TailorJobSelect.Options = list
		state.TailorJobSelect.Refresh()
	}
	if state.PrepJobSelect != nil {
		state.PrepJobSelect.Options = list
		state.PrepJobSelect.Refresh()
	}
}

func tailorProfileAsync(jobDescription string, callback func(string, error)) {
	go func() {
		baseProfileBytes, err := os.ReadFile("references/user-profile.json")
		if err != nil {
			fyne.Do(func() { callback("", err) })
			return
		}

		prompt := fmt.Sprintf(`You are an expert resume writer. Rewrite ONLY the applicant's experience bullet points in the base profile JSON to align with the target job description. All other fields must be preserved verbatim.

Base Profile JSON:
%s

Target Job Description:
%s

Mandates:
1. Rewrite ONLY the experience bullet points. Use the STAR framework (Situation, Task, Action, Result) and begin each bullet with a strong active past-tense verb.
2. Naturally incorporate relevant keywords from the target job description into the rewritten bullets.
3. STRICTLY preserve every quantitative metric (percentages, dollar amounts, team sizes, time periods, dates). Never fabricate, omit, or alter any number.
4. Preserve VERBATIM: personal_info, target_roles, education, projects, skills, and every entry under additional_sections (publications, certifications, awards, etc.). Do not add, remove, reorder, or rename any field. Do not invent skills the applicant did not list. Do not create new sections.
5. Output ONLY valid JSON with the EXACT same set of top-level and nested keys as the input. No explanations, no markdown blocks.`, string(baseProfileBytes), jobDescription)

		tailoredJson, err := callGeminiGo(state.ApiKey, prompt, true)
		if err != nil {
			fyne.Do(func() { callback("", err) })
			return
		}

		tailoredBytes := []byte(tailoredJson)
		if verr := validateTailoredProfile(baseProfileBytes, tailoredBytes); verr != nil {
			fmt.Printf("Tailored profile guard tripped (%v) — falling back to base profile.\n", verr)
			tailoredBytes = baseProfileBytes
		}
		err = writeSecureFile("references/user-profile-tailored.json", tailoredBytes)
		if err != nil {
			fyne.Do(func() { callback("", err) })
			return
		}

		diffText, err := RunTailorResume("references/user-profile.json", "references/user-profile-tailored.json")
		fyne.Do(func() {
			callback(diffText, err)
		})
	}()
}

// 5. INTERVIEW PREPARATION VIEW
func buildPrepTab() fyne.CanvasObject {
	state.PrepJobSelect = widget.NewSelect([]string{}, nil)
	updateDropdownSelectors()

	state.PrepStatus = widget.NewLabel("Generate customized study materials based on company profile and role description.")

	state.CardQuestion = widget.NewLabelWithStyle("Question", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	state.CardAnswer = widget.NewLabelWithStyle("Answer strategy will appear here...", fyne.TextAlignCenter, fyne.TextStyle{})
	state.CardIndicator = widget.NewLabel("0 / 0")

	state.CardAnswer.Hide()

	flipBtn := widget.NewButtonWithIcon("Flip Card", theme.ViewRefreshIcon(), func() {
		if state.CardAnswer.Hidden {
			state.CardAnswer.Show()
		} else {
			state.CardAnswer.Hide()
		}
	})

	prevBtn := widget.NewButton("< Previous", func() {
		if len(state.Flashcards) == 0 {
			return
		}
		state.CurrentCardIdx--
		if state.CurrentCardIdx < 0 {
			state.CurrentCardIdx = len(state.Flashcards) - 1
		}
		showCard(state.CurrentCardIdx)
	})

	nextBtn := widget.NewButton("Next >", func() {
		if len(state.Flashcards) == 0 {
			return
		}
		state.CurrentCardIdx++
		if state.CurrentCardIdx >= len(state.Flashcards) {
			state.CurrentCardIdx = 0
		}
		showCard(state.CurrentCardIdx)
	})

	state.FlashcardBox = container.NewVBox(
		widget.NewCard("Flashcard Deck Player", "", container.NewVBox(
			state.CardQuestion,
			widget.NewSeparator(),
			state.CardAnswer,
			container.NewCenter(flipBtn),
		)),
		container.NewHBox(prevBtn, layout.NewSpacer(), state.CardIndicator, layout.NewSpacer(), nextBtn),
	)
	state.FlashcardBox.Hide()

	startBtn := widget.NewButtonWithIcon("Start Coaching & Prep", theme.MailSendIcon(), func() {
		if state.PrepJobSelect.Selected == "" {
			dialog.ShowInformation("Validation", "Please select a job application to prepare for.", state.Window)
			return
		}

		progress := dialog.NewProgressInfinite("Generating Prep Materials", "Analyzing company role details and compiling Anki flashcards...", state.Window)
		progress.Show()

		go func() {
			parts := strings.SplitN(state.PrepJobSelect.Selected, " - ", 2)
			comp := parts[0]
			role := ""
			if len(parts) == 2 {
				role = parts[1]
			}

			mockData := InterviewPrepData{
				CompanyProfile: fmt.Sprintf("Preparing for %s role at %s. Focus on behavioral STAR method.", role, comp),
				ElevatorPitch:  fmt.Sprintf("Hi, I am Roberto. I have experience in product management and optimization metrics. Ready to join %s.", comp),
				Achievements:   []string{"Launched new payment integration", "Optimized checkout workflow metrics"},
				QuestionsToAsk: []string{"How does team prioritization work?", "What does success look like in this role?"},
				Flashcards: []Flashcard{
					{Question: "What is your elevator pitch?", Answer: "30-second summary introducing yourself, aligning qualifications with company goals."},
					{Question: fmt.Sprintf("Why %s?", comp), Answer: "Reference specific company values, products, or recent milestones you resonate with."},
					{Question: "Tell me about a time you resolved conflict.", Answer: "Describe a context, the disagreement, actions you took to compromise, and the successful resolution (STAR)."},
					{Question: "How do you prioritize features?", Answer: "Discuss frameworks like RICE (Reach, Impact, Confidence, Effort) or ROI assessments."},
				},
			}

			jsonData, _ := json.MarshalIndent(mockData, "", "  ")
			dataPath := "outputs/temp_gui_prep_data.json"
			_ = os.WriteFile(dataPath, jsonData, 0644)

			outText, err := RunPrepareInterview(dataPath, "all")
			os.Remove(dataPath)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}

				state.Flashcards = mockData.Flashcards
				state.CurrentCardIdx = 0
				showCard(0)

				state.PrepStatus.SetText(outText)
				state.FlashcardBox.Show()
			})
		}()
	})

	content := container.NewVBox(
		canvas.NewText("Interview Preparation Coach", theme.PrimaryColor()),
		widget.NewLabel("Prepare behavioral stories and study role-specific flashcards."),
		container.New(layout.NewFormLayout(), widget.NewLabel("Target Job"), state.PrepJobSelect),
		startBtn,
		state.PrepStatus,
		state.FlashcardBox,
	)

	return container.NewScroll(content)
}

func showCard(idx int) {
	if len(state.Flashcards) == 0 {
		return
	}
	card := state.Flashcards[idx]
	state.CardQuestion.SetText(card.Question)
	state.CardAnswer.SetText(card.Answer)
	state.CardAnswer.Hide()
	state.CardIndicator.SetText(fmt.Sprintf("%d / %d", idx+1, len(state.Flashcards)))
}

// 6. SETTINGS VIEW
func buildSettingsTab() fyne.CanvasObject {
	state.SettingsApiKey = widget.NewPasswordEntry()
	state.SettingsEmail = widget.NewEntry()
	state.SettingsPassword = widget.NewPasswordEntry()
	state.SettingsImapServer = widget.NewEntry()
	state.SettingsSaveFolder = widget.NewEntry()

	state.SettingsApiKey.SetText(state.ApiKey)
	state.SettingsEmail.SetText(state.Email)
	state.SettingsPassword.SetText(state.Password)
	state.SettingsImapServer.SetText(state.ImapServer)
	state.SettingsSaveFolder.SetText(state.SaveFolder)

	state.SettingsApiModel = widget.NewSelect([]string{
		"gemini-3.1-flash-lite",
		"gemini-2.5-flash",
		"gemini-1.5-flash",
		"gemini-1.5-pro",
	}, func(selected string) {
		state.ApiModel = selected
	})
	if state.ApiModel == "" {
		state.ApiModel = "gemini-3.1-flash-lite"
	}
	state.SettingsApiModel.SetSelected(state.ApiModel)

	saveBtn := widget.NewButtonWithIcon("Save Configurations", theme.DocumentSaveIcon(), func() {
		state.ApiKey = state.SettingsApiKey.Text
		state.ApiModel = state.SettingsApiModel.Selected
		state.Email = state.SettingsEmail.Text
		state.Password = state.SettingsPassword.Text
		state.ImapServer = state.SettingsImapServer.Text
		state.SaveFolder = state.SettingsSaveFolder.Text

		err := saveConfigurations()
		if err != nil {
			dialog.ShowError(err, state.Window)
			return
		}
		dialog.ShowInformation("Success", "Settings written to secure local .env configuration.", state.Window)
	})

	wizardBtn := widget.NewButtonWithIcon("Run Setup Wizard", theme.DocumentCreateIcon(), func() {
		showOnboardingWizard()
	})

	// Save Folder field uses the same Quick-Jump / semantic folder picker as
	// the rest of the app so users don't have to type filesystem paths by hand.
	// The underlying Entry is kept (read-only) so the existing save logic that
	// reads state.SettingsSaveFolder.Text continues to work unchanged.
	state.SettingsSaveFolder.Disable()
	browseFolderBtn := widget.NewButtonWithIcon("Browse / Create…", theme.FolderOpenIcon(), func() {
		showCustomFolderPicker(state.Window, "Choose Save Folder", false, func(picked string) {
			state.SettingsSaveFolder.Enable()
			state.SettingsSaveFolder.SetText(picked)
			state.SettingsSaveFolder.Disable()
		})
	})
	saveFolderRow := container.NewBorder(nil, nil, nil, browseFolderBtn, state.SettingsSaveFolder)

	// Email / IMAP credential fields are temporarily hidden from the settings
	// form while the email-sync feature is disabled. The underlying widgets and
	// persisted values are retained so the feature can be re-enabled later.
	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Gemini API Key"), state.SettingsApiKey,
		widget.NewLabel("Gemini API Model"), state.SettingsApiModel,
		widget.NewLabel("Save Folder"), saveFolderRow,
	)

	githubBtn := widget.NewButtonWithIcon("GitHub: /bot-bbio", theme.HelpIcon(), func() {
		openLink("https://github.com/bot-bbio")
	})
	linkedinBtn := widget.NewButtonWithIcon(linkedInCreditDisplay, theme.HelpIcon(), func() {
		openLink(linkedInCreditURL)
	})
	creditsRow := container.NewHBox(
		widget.NewLabel("Credits:"),
		githubBtn,
		linkedinBtn,
	)

	content := container.NewVBox(
		canvas.NewText("Settings & Configurations", theme.PrimaryColor()),
		widget.NewLabel("Manage local API keys and configurations securely stored in your .env file."),
		form,
		container.NewHBox(saveBtn, wizardBtn),
		widget.NewSeparator(),
		creditsRow,
	)

	return container.NewScroll(content)
}

// onboardingGuideText returns the markdown shown in the Help tab's Get Started
// section. It reflects only the currently active features.
func onboardingGuideText() string {
	return "### 🚀 Get Started with LeGaJ\n" +
		"Follow this step-by-step workflow to maximize your job application success:\n\n" +
		"* **Step 1: Setup & API Keys**\n" +
		"  Head over to the **Settings** tab and enter your *Gemini API Key*. Save configurations to store them securely. You can also run the *Setup Wizard* to initialize target folders.\n" +
		"* **Step 2: Initialize Your Profile**\n" +
		"  Upload your existing resume (PDF/DOCX) or fill in details in the **Base Profile** tab. This forms your main resume reference.\n" +
		"* **Step 3: Collect Job Leads**\n" +
		"  Clip listings from job boards (LinkedIn, Indeed, etc.) directly into your **Job Leads** inbox using our browser tools. Review them and add the ones you like to your tracker.\n" +
		"* **Step 4: Track Your Applications**\n" +
		"  Use the **Job Tracker** to manage every application's status, open saved documents, and keep your search organized from wishlist to offer.\n\n" +
		"### ✅ Best Practices\n" +
		"* **Recommended Model**\n" +
		"  Use **gemini-3.1-flash-lite** as your default model. It offers the highest rate limits and quotas plus the fastest response times, which matters most during bulk operations.\n" +
		"* **Mass Application Workflows**\n" +
		"  * *Clipper Workflow*: Clip job postings straight into your **Job Leads** inbox as you browse, then review and add the strongest matches to your tracker in one pass.\n" +
		"  * *CSV Workflow*: Build a CSV file with job URLs in the first column, load them all at once with the **Bulk Import** button in the Job Tracker, then update statuses as your applications progress."
}

// 7. HELP & DOCUMENTATION VIEW
// generateBookmarkletJS returns the clipper bookmarklet JavaScript formatted
// with the active clip server port and auth token. Shared by the Help tab and
// the setup wizard so both stay in sync.
func generateBookmarkletJS() string {
	return fmt.Sprintf(`javascript:(function(){
  var h=window.location.hostname,p=window.location.href,c='',r='',l='',d='';
  try{
    if(h.includes('linkedin.com')){
      var t=document.title.replace(' | LinkedIn','').split(' at ');
      c=(document.querySelector('[data-tracking-control-name="public_jobs_topcard-org-name"]')||document.querySelector('.topcard__org-name-link')||document.querySelector('.job-details-jobs-unified-top-card__company-name a')||{innerText:t[1]||''}).innerText;
      r=(document.querySelector('h1.t-24')||document.querySelector('.topcard__title')||document.querySelector('.job-details-jobs-unified-top-card__job-title h1')||{innerText:t[0]||''}).innerText;
      l=(document.querySelector('.topcard__flavor--bullet')||document.querySelector('.job-details-jobs-unified-top-card__bullet')||{innerText:''}).innerText;
      d=(document.querySelector('.description__text')||document.querySelector('#job-details')||{innerText:''}).innerText;
    } else if(h.includes('indeed.com')){
      var t2=document.title.replace(' - Indeed','').split(' at ');
      r=(document.querySelector('[data-testid="jobsearch-JobInfoHeader-title"]')||document.querySelector('h1.jobsearch-JobInfoHeader-title')||{innerText:t2[0]||''}).innerText;
      c=(document.querySelector('[data-company-name]')||document.querySelector('[data-testid="inlineHeader-companyName"]')||{innerText:t2[1]||''}).innerText;
      l=(document.querySelector('[data-testid="inlineHeader-companyLocation"]')||{innerText:''}).innerText;
      d=(document.querySelector('#jobDescriptionText')||{innerText:''}).innerText;
    } else if(h.includes('greenhouse.io')||h.includes('grnh.se')){
      var t3=document.title.split(' - ');
      r=(document.querySelector('h1.app-title')||document.querySelector('.app__title h1')||{innerText:t3[0]||''}).innerText;
      c=(document.querySelector('.company-name')||{innerText:t3[t3.length-1]||''}).innerText;
      l=(document.querySelector('.location')||{innerText:''}).innerText;
      d=(document.querySelector('#content')||{innerText:''}).innerText;
    } else if(h.includes('lever.co')){
      var t4=document.title.split('|');
      r=(document.querySelector('h2')||{innerText:t4[0]||''}).innerText;
      c=t4.length>1?t4[t4.length-1].trim():h.split('.')[0];
      l=(document.querySelector('.sort-by-time.posting-category')||{innerText:''}).innerText;
      d=(document.querySelector('.section-wrapper')||{innerText:''}).innerText;
    } else if(h.includes('myworkdayjobs.com')||h.includes('workday.com')){
      var t5=document.title.split('|');
      r=(document.querySelector('[data-automation-id="jobPostingHeader"]')||{innerText:t5[0]||''}).innerText;
      c=t5.length>1?t5[t5.length-1].trim():h.split('.')[0];
      l=(document.querySelector('[data-automation-id="locations"]')||{innerText:''}).innerText;
      d=(document.querySelector('[data-automation-id="jobPostingDescription"]')||{innerText:''}).innerText;
    } else if(h.includes('ashbyhq.com')){
      var t6=document.title.split(' at ');
      r=(document.querySelector('h1')||{innerText:t6[0]||''}).innerText;
      c=t6.length>1?t6[t6.length-1].trim():h.split('.')[0];
      l=(document.querySelector('.ashby-job-posting-brief-location')||{innerText:''}).innerText;
      d=(document.querySelector('.ashby-job-posting-description')||{innerText:''}).innerText;
    } else if(h.includes('icims.com')){
      var t7=document.title.split('-');
      r=(document.querySelector('h1')||{innerText:t7[0]||''}).innerText;
      c=t7.length>1?t7[t7.length-1].trim():h.split('.')[0];
      l=(document.querySelector('.iCIMS_InfoMsg')||{innerText:''}).innerText;
      d=(document.querySelector('#jobDetails')||{innerText:''}).innerText;
    }

    if(!r){
      var sel=['.job-title','.posting-title','.title','.role','h1'];
      for(var i=0;i<sel.length;i++){
        var el=document.querySelector(sel[i]);
        if(el&&el.innerText.trim()){r=el.innerText.trim();break;}
      }
      if(!r){
        var og=document.querySelector('meta[property="og:title"]');
        if(og&&og.content.trim())r=og.content.trim();
      }
      if(!r&&document.title){
        var ct=document.title.replace(/\s*[-|•|\|]\s*(LinkedIn|Indeed|ZipRecruiter|Glassdoor|Google).*$/i,'').trim();
        r=ct;
      }
      if(!r||r.toLowerCase().includes('apply now')||r.toLowerCase().includes('current openings')||r.toLowerCase().includes('career page')||r.toLowerCase().includes('job details')){
        var segs=window.location.pathname.split('/').filter(Boolean);
        for(var j=0;j<segs.length;j++){
          var s=segs[j];
          if(s.length>5&&!s.includes('.')&&isNaN(s)){
            r=decodeURIComponent(s).replace(/[-_]/g,' ').replace(/\b\w/g,function(x){return x.toUpperCase();});
            break;
          }
        }
      }
    }

    if(!c){
      var ht=document.title;
      c=ht.includes(' at ')?ht.split(' at ').pop().split('|')[0].split('-')[0].trim():'';
    }

    if(!l){
      var els=document.querySelectorAll('*');
      for(var i=0;i<els.length;i++){
        var el=els[i];
        if(el.children.length===0&&el.innerText.trim()){
          var idc=(el.id+' '+el.className).toLowerCase();
          if(idc.includes('location')||idc.includes('workplace')||idc.includes('remote')||idc.includes('hybrid')){
            var txt=el.innerText.trim();
            if(txt.length>2&&txt.length<100){l=txt;break;}
          }
        }
      }
      if(!l){
        var geoRegex=/^[A-Z][a-zA-Z\s.]+,\s*[A-Z]{2}(\s+[A-Z][a-zA-Z\s.]+)?$/;
        var tags=document.querySelectorAll('h2, h3, h4, p, span');
        for(var i=0;i<tags.length;i++){
          var txt=tags[i].innerText.trim();
          if(geoRegex.test(txt)||txt.toLowerCase()==='remote'||txt.toLowerCase().includes('remote,')||txt.toLowerCase().includes('hybrid,')){
            l=txt;
            break;
          }
        }
      }
    }

    if(!d){
      var selD=['main','article','[class*="description"]','[id*="description"]','#job-details','#content','.section-wrapper','[data-automation-id="jobPostingDescription"]','.ashby-job-posting-description','#jobDescriptionText'];
      for(var i=0;i<selD.length;i++){
        var el=document.querySelector(selD[i]);
        if(el&&el.innerText.trim()){d=el.innerText.trim();break;}
      }
      if(!d)d=document.body.innerText;
    }
  } catch(e){}
  c=(c||'').trim().substring(0,100);
  r=(r||'').trim().substring(0,150);
  l=(l||'').trim().substring(0,100);
  d=(d||'').trim().substring(0,50000);
  if(!c&&!r){return;}
  fetch('http://127.0.0.1:%d/clip',{method:'POST',headers:{'Content-Type':'application/json','X-LeGaJ-Token':'%s'},body:JSON.stringify({company:c,role:r||document.title,location:l,link:p,description:d})})
    .then(function(res){return res.json();})
    .then(function(){alert('\u2705 Clipped! Check your Job Leads in LeGaJ.');})
    .catch(function(e){
      var url='http://127.0.0.1:%d/clip?token=%s&company='+encodeURIComponent(c)+'&role='+encodeURIComponent(r||document.title)+'&location='+encodeURIComponent(l)+'&link='+encodeURIComponent(p)+'&description='+encodeURIComponent(d);
      var w=window.open(url,'_blank','width=500,height=400,status=no,menubar=no,toolbar=no');
      if(!w){alert('\u274C Clip failed: Popup was blocked!\n\nTo allow job clipping on this site:\n1. Click the blocked popup icon in your browser address bar.\n2. Select \"Always allow popups and redirects from \" + window.location.origin + \".\n3. Click Done and try clipping again.\n\nAlso make sure LeGaJ is running and open.');}
    });
})();`, activePort, clipAuthToken, activePort, clipAuthToken)
}

func buildHelpTab() fyne.CanvasObject {
	// Create markdown widgets for each documentation section
	onboardingRichText := widget.NewRichTextFromMarkdown(onboardingGuideText())
	onboardingRichText.Wrapping = fyne.TextWrapWord

	// Bookmarklet configuration
	bookmarkletJs := generateBookmarkletJS()

	bookmarkletEntry := widget.NewEntry()
	bookmarkletEntry.SetText(bookmarkletJs)
	bookmarkletEntry.MultiLine = false

	copyBtn := widget.NewButtonWithIcon("Copy Bookmarklet Code", theme.ContentCopyIcon(), func() {
		state.Window.Clipboard().SetContent(bookmarkletJs)
		dialog.ShowInformation("Copied!", "Bookmarklet code copied to clipboard.", state.Window)
	})
	copyBtn.Importance = widget.HighImportance

	bookmarkletMd := "### 📎 Instant Job Clipper\n" +
		"The LeGaJ bookmarklet allows you to instantly save job listings from LinkedIn, Indeed, Greenhouse, Lever, Workday, Ashby, and iCIMS directly to your local application inbox.\n\n" +
		"#### 🔧 How to Install\n" +
		"1. Click the **Copy Bookmarklet Code** button below to copy the javascript setup.\n" +
		"2. Ensure your browser's bookmarks bar is visible (\\x60Ctrl+Shift+B\\x60 or \\x60Cmd+Shift+B\\x60).\n" +
		"3. Right-click the bookmarks bar and select **Add page** or **Add bookmark**.\n" +
		"4. Set the name to **📎 Clip to LeGaJ** and paste the copied code into the **URL / Address** field.\n" +
		"5. Save the bookmark.\n\n" +
		"#### 🖱️ How to Use\n" +
		"Simply click the **📎 Clip to LeGaJ** bookmark while viewing a job description on any supported job site. The application details (role title, company, description, and link) will instantly clip into your local Inbox."

	bookmarkletRichText := widget.NewRichTextFromMarkdown(bookmarkletMd)
	bookmarkletRichText.Wrapping = fyne.TextWrapWord

	bookmarkletBox := container.NewVBox(
		bookmarkletRichText,
		widget.NewSeparator(),
		container.NewHBox(copyBtn, layout.NewSpacer()),
		widget.NewLabel("Advanced: bookmark URL source code (for manual copy):"),
		bookmarkletEntry,
	)

	// Chrome Extension
	absExtensionPath, _ := filepath.Abs("extension")
	extensionMd := fmt.Sprintf("### 🧩 Chrome & Edge Extension\n"+
		"If a site's Content Security Policy blocks the bookmarklet, install our lightweight Chrome/Edge extension:\n\n"+
		"1. Open Chrome or Edge and navigate to \\x60chrome://extensions\\x60 or \\x60edge://extensions\\x60.\n"+
		"2. Enable **Developer mode** via the toggle switch in the top-right corner.\n"+
		"3. Click the **Load unpacked** button in the top-left corner.\n"+
		"4. Select the \\x60extension\\x60 directory located inside your LeGaJ installation:\n"+
		"   %s\n"+
		"5. Pin the **LeGaJ Clipper** extension to your browser toolbar.\n"+
		"6. Click the extension icon on any job posting page to clip details instantly.", absExtensionPath)

	extensionRichText := widget.NewRichTextFromMarkdown(extensionMd)
	extensionRichText.Wrapping = fyne.TextWrapWord

	// Troubleshooting & Locations
	troubleshootMd := "### 🔧 Troubleshooting & Locations\n\n" +
		"#### 📂 File & Folder Structure\n" +
		"* **\\x60references/user-profile.json\\x60**: Holds your editable base profile.\n" +
		"* **\\x60references/job-tracker.json\\x60**: Contains your tracked job applications.\n" +
		"* **\\x60outputs/\\x60**: Folder where generated PDFs are compiled.\n\n" +
		"#### 💡 Common Issues & Fixes\n" +
		"* **Bookmarklet not responding?** Make sure LeGaJ is open and running. Check browser address bar for blocked pop-ups.\n" +
		"* **HTTPS/Mixed Content error?** Modern browsers restrict local HTTP connections from HTTPS web pages. Use the unpacked Chrome/Edge extension.\n" +
		"* **ReportLab or GenAnki error?** Ensure Python is installed and run \\x60pip install -r requirements.txt\\x60 to install package dependencies."

	troubleshootRichText := widget.NewRichTextFromMarkdown(troubleshootMd)
	troubleshootRichText.Wrapping = fyne.TextWrapWord

	// Security & Ethics Disclosure
	securityMd := "### 🔒 Security, Privacy & Ethics\n" +
		"AI and web scraping come with security risks, particularly prompt injections. LeGaJ is designed to protect your privacy:\n\n" +
		"* **Local-First Processing**: Your resume contents, personal info (PII), and application histories remain strictly on your local disk.\n" +
		"* **Direct API Connections**: Your Gemini API key is stored in your local \\x60.env\\x60 and sent directly to Google. No third-party servers act as an intermediary.\n" +
		"* **No Auto-Submit**: LeGaJ never automatically submits applications. It only prepares draft PDFs and tracks logs, keeping you in complete control."

	securityRichText := widget.NewRichTextFromMarkdown(securityMd)
	securityRichText.Wrapping = fyne.TextWrapWord

	// Accordion layout. Each section's body is wrapped in NewPadded so the
	// markdown doesn't crowd the accordion frame on either side (Bug 11).
	accordion := widget.NewAccordion(
		widget.NewAccordionItem("1. Step-by-Step Onboarding Guide", container.NewPadded(onboardingRichText)),
		widget.NewAccordionItem("2. Browser Clipper Bookmarklet", container.NewPadded(bookmarkletBox)),
		widget.NewAccordionItem("3. Chrome/Edge Extension Setup", container.NewPadded(extensionRichText)),
		widget.NewAccordionItem("4. Troubleshooting & File Locations", container.NewPadded(troubleshootRichText)),
		widget.NewAccordionItem("5. Security & Ethics Disclosure", container.NewPadded(securityRichText)),
	)
	accordion.MultiOpen = true
	accordion.Open(0)

	// Header and title
	titleText := canvas.NewText("Help & Documentation", theme.PrimaryColor())
	titleText.TextSize = 20
	titleText.TextStyle = fyne.TextStyle{Bold: true}

	// Verbatim Warning label at the very top of the help page
	warningLabel := widget.NewLabel("AI and the Internet are inherently dangerous. Any tool that inputs unverified information from the web is vulnerable to prompt injection. Use at your own risk.")
	warningLabel.Wrapping = fyne.TextWrapWord

	githubBtn := widget.NewButtonWithIcon("GitHub: /bot-bbio", theme.HelpIcon(), func() {
		openLink("https://github.com/bot-bbio")
	})
	linkedinBtn := widget.NewButtonWithIcon(linkedInCreditDisplay, theme.HelpIcon(), func() {
		openLink(linkedInCreditURL)
	})
	creditsRow := container.NewHBox(
		widget.NewLabel("Credits:"),
		githubBtn,
		linkedinBtn,
	)

	content := container.NewVBox(
		container.NewVBox(
			titleText,
			widget.NewLabel("Follow instructions and troubleshoot issues here."),
			warningLabel,
		),
		widget.NewSeparator(),
		container.NewPadded(accordion),
		widget.NewSeparator(),
		creditsRow,
	)

	return container.NewScroll(container.NewPadded(content))
}

// ensureGenericCoverLetter acts as a backend-only skill to generate and cache a generic cover letter template
// based on the user's profile and desired tone from @Personal-labour-mobile examples.
func ensureGenericCoverLetter(apiKey string, profileBytes []byte) (string, error) {
	templatePath := "references/generic_cover_letter_template.txt"
	if _, err := os.Stat(templatePath); err == nil {
		content, err := os.ReadFile(templatePath)
		if err == nil && len(content) > 0 {
			return string(content), nil
		}
	}

	// Create references dir if not exists
	os.MkdirAll("references", os.ModePerm)

	prompt := fmt.Sprintf(`As an expert career coach, write a generic, versatile cover letter template with placeholder bracketed variables (like [Full Name], [Target Role], [Target Company], etc.) based on the following profile.
The style MUST heavily mirror this example template:
"""
To Whom it May Concern,

My name is [Full Name] and I write to you regarding the [Target Role] role at [Target Company]. My background in [Fields of Experience], paired with a focus on [Specialized Skill/Focus], aligns perfectly with [Target Company]’s mission to [Target Company Mission/Value]. I am eager to transition into a high-impact environment where I can [Eager Transition/Impact Goal].

In my current role as a [Current Role] at [Current Company], I manage [Current Responsibilities], consistently [Key Achievement/Metric]. I have focused on driving growth through [Action/Focus Area], a direct parallel to the [Target Area/Skill] [Target Company] seeks. Previously, at [Previous Company 1], I led [Previous Responsibilities 1], gaining deep familiarity with the [Industry Standard Processes] and the industry-standard tools used today.

My experience as a [Previous Role 2] at [Previous Company 2] demonstrated my ability to lead through significant transformation. Overseeing a [Quantifiable Asset/Portfolio Value] portfolio, I overhauled our [Pipelines/Infrastructure] during a critical period. This self-directed leadership in a high-pressure environment reflects the entrepreneurial mindset necessary to [Desired Mindset Action/Goal]. I am comfortable bridging the gap between [technical/operational requirements] and [user/client adoption] to ensure high-performing features deliver real value.

I am confident that my blend of [Key Skill 1], [Key Skill 2], and [Key Skill 3] will make me a strong asset to the [Target Team/Department] team. I look forward to discussing this opportunity further and I hope to hear from you soon!

Sincerely,

[Full Name]
"""

Create a slightly generalized version of this that can act as a baseline template for future tailored letters. 
Keep the placeholders (e.g., [Full Name], [Target Role], [Target Company], [Fields of Experience], etc.) instead of substituting specific values, but ensure the core experience highlights match the structure of the provided profile.

Profile:
%s

Output ONLY the cover letter text.`, string(profileBytes))

	templateText, err := callGeminiGo(apiKey, prompt, false)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(templatePath, []byte(templateText), 0644)
	if err != nil {
		fmt.Println("Warning: Failed to save generic cover letter template:", err)
	}

	return templateText, nil
}

// writeDefaultCoverLetterTemplate writes the default template and compiles the PDF
func writeDefaultCoverLetterTemplate() error {
	defaultTemplate := `To Whom it May Concern,

My name is [Full Name] and I write to you regarding the [Target Role] role at [Target Company]. My background in [Fields of Experience], paired with a focus on [Specialized Skill/Focus], aligns perfectly with [Target Company]’s mission to [Target Company Mission/Value]. I am eager to transition into a high-impact environment where I can [Eager Transition/Impact Goal].

In my current role as a [Current Role] at [Current Company], I manage [Current Responsibilities], consistently [Key Achievement/Metric]. I have focused on driving growth through [Action/Focus Area], a direct parallel to the [Target Area/Skill] [Target Company] seeks. Previously, at [Previous Company 1], I led [Previous Responsibilities 1], gaining deep familiarity with the [Industry Standard Processes] and the industry-standard tools used today.

My experience as a [Previous Role 2] at [Previous Company 2] demonstrated my ability to lead through significant transformation. Overseeing a [Quantifiable Asset/Portfolio Value] portfolio, I overhauled our [Pipelines/Infrastructure] during a critical period. This self-directed leadership in a high-pressure environment reflects the entrepreneurial mindset necessary to [Desired Mindset Action/Goal]. I am comfortable bridging the gap between [technical/operational requirements] and [user/client adoption] to ensure high-performing features deliver real value.

I am confident that my blend of [Key Skill 1], [Key Skill 2], and [Key Skill 3] will make me a strong asset to the [Target Team/Department] team. I look forward to discussing this opportunity further and I hope to hear from you soon!

Sincerely,

[Full Name]`

	os.MkdirAll("references", 0755)
	err := os.WriteFile("references/generic_cover_letter_template.txt", []byte(defaultTemplate), 0644)
	if err != nil {
		return err
	}
	os.MkdirAll("outputs", 0755)
	_, err = RunGenerateCoverLetter("references/user-profile.json", "references/generic_cover_letter_template.txt", "outputs/generic_cover_letter_template.pdf")
	return err
}

// generateCustomCoverLetterTemplate calls Gemini to build a custom cover letter template by genericizing the user's pasted cover letter
func generateCustomCoverLetterTemplate(apiKey string, originalLetter string) (string, error) {
	prompt := fmt.Sprintf(`As an expert career coach, write a generic, versatile cover letter template with placeholder bracketed variables (like [Full Name], [Target Role], [Target Company], [Fields of Experience], [Recent Role], [Recent Company], [Quantifiable Assets/Value], etc.) based on the user's original cover letter text.

The style, structure, tone, and paragraph layout MUST heavily mirror the user's original letter:
"""
%s
"""

Convert this original letter into a generic baseline template. Replace all specific PII (names, emails, phone numbers, locations), specific company names, specific role titles, specific dates, and highly specific project names with clear placeholder bracketed variables (e.g. [Full Name], [Target Company], [Target Role], [Fields of Experience], [Recent Role], [Recent Company], [Quantifiable Assets/Value], etc.), while retaining the underlying sentence structures, tone, and flow.

Output ONLY the cover letter template text.`, originalLetter)

	templateText, err := callGeminiGo(apiKey, prompt, false)
	if err != nil {
		return "", err
	}

	os.MkdirAll("references", 0755)
	err = os.WriteFile("references/generic_cover_letter_template.txt", []byte(templateText), 0644)
	if err != nil {
		return "", err
	}

	os.MkdirAll("outputs", 0755)
	_, err = RunGenerateCoverLetter("references/user-profile.json", "references/generic_cover_letter_template.txt", "outputs/generic_cover_letter_template.pdf")
	if err != nil {
		return "", err
	}

	return templateText, nil
}

// getWindowsDrives returns all available drive letters on Windows
func getWindowsDrives() []string {
	var drives []string
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		driveStr := string(drive) + ":\\"
		if _, err := os.Stat(driveStr); err == nil {
			drives = append(drives, driveStr)
		}
	}
	return drives
}

// showCustomFilePicker opens a custom folder/file selector dialog window styled like the File Manager tab.
// offerWorkspaceSetup runs after a successful résumé parse. When the user has
// not yet chosen a real Save Folder, it suggests creating a LeGaJ Workspace at
// ~/Documents/LeGaJ Workspace and copying the source résumé there for safe
// keeping. When the workspace is already configured, it just shows a brief
// success notice. The actual work runs on the main thread via fyne.Do so the
// dialog hierarchy is consistent with the rest of the import flow.
func offerWorkspaceSetup(sourceResumePath string) {
	needsSetup := state.SaveFolder == "" || state.SaveFolder == "." || state.SaveFolder == "outputs"
	fyne.Do(func() {
		if !needsSetup {
			dialog.ShowInformation("Parse Complete", "Successfully parsed and structured résumé using Gemini AI!", state.Window)
			return
		}
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			dialog.ShowInformation("Parse Complete", "Successfully parsed and structured résumé using Gemini AI!\n\n(Could not locate your home folder to suggest a workspace — set one manually in Settings.)", state.Window)
			return
		}
		suggested := filepath.Join(home, "Documents", "LeGaJ Workspace")
		msg := fmt.Sprintf("Résumé parsed successfully.\n\nCreate a LeGaJ Workspace at:\n  %s\n\nA copy of your résumé will be saved there, and all generated résumés and cover letters will be stored in this folder.\n\nCreate workspace and save a copy now? (Recommended)", suggested)
		dialog.ShowConfirm("Set Up LeGaJ Workspace", msg, func(ok bool) {
			if !ok {
				return
			}
			if err := os.MkdirAll(suggested, 0755); err != nil {
				dialog.ShowError(fmt.Errorf("could not create workspace folder: %v", err), state.Window)
				return
			}
			if copyErr := copyFileTo(sourceResumePath, suggested); copyErr != nil {
				// Non-fatal: workspace was created; surface the copy failure
				// but keep the workspace configured.
				dialog.ShowError(fmt.Errorf("workspace created, but copying résumé failed: %v", copyErr), state.Window)
			}
			state.SaveFolder = suggested
			if saveErr := saveConfigurations(); saveErr != nil {
				dialog.ShowError(fmt.Errorf("workspace created, but saving settings failed: %v", saveErr), state.Window)
				return
			}
			dialog.ShowInformation("Workspace Ready", fmt.Sprintf("LeGaJ Workspace is set to:\n  %s\n\nYou can change this in Settings.", suggested), state.Window)
		}, state.Window)
	})
}

// copyFileTo copies src into destDir, keeping the original filename. Returns
// an error if src cannot be opened, destDir cannot be written to, or the copy
// itself fails.
func copyFileTo(src, destDir string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	dest := filepath.Join(destDir, filepath.Base(src))
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// knownFolder is a one-click shortcut surfaced at the top of the file picker.
// Path is empty when the folder is not present on the current machine — those
// entries are filtered out before rendering.
type knownFolder struct {
	Label string
	Path  string
}

// resolveKnownFolders enumerates the shortcuts the picker offers in its
// "Quick Jump" row. Order matters: most-likely targets first. Folders that do
// not exist on the current machine (e.g. OneDrive on a non-OneDrive box) are
// dropped so we never surface a dead shortcut.
func resolveKnownFolders() []knownFolder {
	home, _ := os.UserHomeDir()
	candidates := []knownFolder{
		{Label: "Home", Path: home},
		{Label: "Desktop", Path: filepath.Join(home, "Desktop")},
		{Label: "Documents", Path: filepath.Join(home, "Documents")},
		{Label: "Downloads", Path: filepath.Join(home, "Downloads")},
		{Label: "OneDrive", Path: filepath.Join(home, "OneDrive")},
		{Label: "OneDrive Documents", Path: filepath.Join(home, "OneDrive", "Documents")},
		{Label: "OneDrive Desktop", Path: filepath.Join(home, "OneDrive", "Desktop")},
	}
	// Append LeGaJ workspace if it has been configured to a real folder
	// (i.e. not the default "outputs" / "." sentinel paths).
	if state.SaveFolder != "" && state.SaveFolder != "outputs" && state.SaveFolder != "." {
		if info, err := os.Stat(state.SaveFolder); err == nil && info.IsDir() {
			candidates = append(candidates, knownFolder{Label: "LeGaJ Workspace", Path: state.SaveFolder})
		}
	}

	var out []knownFolder
	seen := make(map[string]bool)
	for _, k := range candidates {
		if k.Path == "" || seen[k.Path] {
			continue
		}
		if info, err := os.Stat(k.Path); err != nil || !info.IsDir() {
			continue
		}
		seen[k.Path] = true
		out = append(out, k)
	}
	return out
}

// matchKnownFolder maps a search keyword to a known-folder path. Returns the
// resolved path and true on match. Matching is prefix-based and
// case-insensitive ("desk" → Desktop, "doc" → Documents, "down" → Downloads,
// "one" → OneDrive, "home" / "user" → Home, "work" / "legaj" → Workspace).
func matchKnownFolder(query string) (string, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "", false
	}
	folders := resolveKnownFolders()
	for _, f := range folders {
		label := strings.ToLower(f.Label)
		if strings.HasPrefix(label, q) || strings.Contains(label, q) {
			return f.Path, true
		}
	}
	// Synonyms for common asks.
	synonyms := map[string]string{
		"user":   "Home",
		"work":   "LeGaJ Workspace",
		"legaj":  "LeGaJ Workspace",
		"cloud":  "OneDrive",
	}
	if target, ok := synonyms[q]; ok {
		for _, f := range folders {
			if f.Label == target {
				return f.Path, true
			}
		}
	}
	return "", false
}

// showCustomFolderPicker is the folder-selection sibling of showCustomFilePicker.
// It reuses the same Quick Jump shortcuts and semantic path-bar typing so the
// experience matches across "pick a file" and "pick a folder" flows. The user
// confirms a directory with the prominent "Select This Folder" button — files
// in the listing are hidden because they are not selectable here.
//
// A "Create LeGaJ Workspace" affordance is offered when the workspace folder
// does not yet exist; this lets the user provision the recommended folder
// directly from the picker instead of having to switch to Explorer.
func showCustomFolderPicker(parentWindow fyne.Window, title string, showTip bool, onSelect func(string)) {
	pickerWin := state.App.NewWindow(title)
	pickerWin.Resize(fyne.NewSize(580, 460))

	currentDir := state.SaveFolder
	if currentDir == "" || currentDir == "outputs" || currentDir == "." {
		if home, err := os.UserHomeDir(); err == nil {
			currentDir = home
		} else {
			currentDir = "."
		}
	}
	currentDir = filepath.Clean(currentDir)

	pathEntry := widget.NewEntry()
	pathEntry.SetText(currentDir)

	explorerBox := container.NewVBox()

	var refreshPicker func()
	refreshPicker = func() {
		if currentDir == "Drives" {
			pathEntry.SetText("My Computer (Drives)")
			explorerBox.Objects = nil
			for _, dr := range getWindowsDrives() {
				drName := dr
				row := container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabel(drName), layout.NewSpacer())
				btn := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() {
					currentDir = drName
					refreshPicker()
				})
				row.Add(btn)
				explorerBox.Add(row)
			}
			explorerBox.Refresh()
			return
		}
		currentDir = filepath.Clean(currentDir)
		pathEntry.SetText(currentDir)

		entries, err := os.ReadDir(currentDir)
		if err != nil {
			explorerBox.Objects = []fyne.CanvasObject{widget.NewLabel(fmt.Sprintf("Error reading dir: %v", err))}
			explorerBox.Refresh()
			return
		}

		explorerBox.Objects = nil
		shown := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
				continue
			}
			name := entry.Name()
			displayName := name
			if len(displayName) > 50 {
				displayName = displayName[:47] + "..."
			}
			row := container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabel(displayName), layout.NewSpacer())
			openBtn := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() {
				currentDir = filepath.Join(currentDir, name)
				refreshPicker()
			})
			row.Add(openBtn)
			explorerBox.Add(row)
			shown++
		}
		if shown == 0 {
			explorerBox.Add(widget.NewLabel("(no subfolders here — use 'Select This Folder' to pick the current location)"))
		}
		explorerBox.Refresh()
	}

	pathEntry.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text == "My Computer (Drives)" || strings.ToLower(text) == "drives" {
			currentDir = "Drives"
			refreshPicker()
			return
		}
		if match, ok := matchKnownFolder(text); ok {
			currentDir = match
			refreshPicker()
			return
		}
		cleanPath := filepath.Clean(text)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			currentDir = cleanPath
			refreshPicker()
		} else {
			dialog.ShowError(fmt.Errorf("Invalid directory or shortcut: %s\n\nTry a path, or a keyword like 'desktop', 'documents', 'downloads', 'home'.", text), pickerWin)
			pathEntry.SetText(currentDir)
		}
	}

	backBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if currentDir == "Drives" {
			return
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			if len(getWindowsDrives()) > 0 {
				currentDir = "Drives"
			}
			refreshPicker()
		} else {
			currentDir = parent
			refreshPicker()
		}
	})
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { refreshPicker() })

	pathRow := container.NewBorder(nil, nil, container.NewHBox(backBtn, refreshBtn), nil, pathEntry)

	// Quick Jump shortcuts mirror the file picker so users can hop between
	// common locations without manual navigation.
	shortcutsRow := container.NewHBox()
	for _, f := range resolveKnownFolders() {
		target := f.Path
		btn := widget.NewButtonWithIcon(f.Label, theme.FolderIcon(), func() {
			currentDir = target
			refreshPicker()
		})
		shortcutsRow.Add(btn)
	}
	if drives := getWindowsDrives(); len(drives) > 0 {
		btn := widget.NewButtonWithIcon("Drives", theme.ComputerIcon(), func() {
			currentDir = "Drives"
			refreshPicker()
		})
		shortcutsRow.Add(btn)
	}

	selectThisBtn := widget.NewButtonWithIcon("Select This Folder", theme.ConfirmIcon(), func() {
		if currentDir == "Drives" {
			dialog.ShowInformation("Pick a Folder", "Open a drive first, then choose a folder inside it.", pickerWin)
			return
		}
		onSelect(currentDir)
		pickerWin.Close()
	})
	selectThisBtn.Importance = widget.HighImportance

	createWorkspaceBtn := widget.NewButtonWithIcon("Create LeGaJ Workspace Here", theme.ContentAddIcon(), func() {
		if currentDir == "Drives" {
			dialog.ShowError(fmt.Errorf("open a folder first"), pickerWin)
			return
		}
		ws := filepath.Join(currentDir, "LeGaJ Workspace")
		if err := os.MkdirAll(ws, 0755); err != nil {
			dialog.ShowError(fmt.Errorf("could not create workspace folder: %v", err), pickerWin)
			return
		}
		currentDir = ws
		refreshPicker()
	})

	actionRow := container.NewHBox(selectThisBtn, createWorkspaceBtn, layout.NewSpacer())

	toolbar := container.NewVBox(
		pathRow,
		container.NewHScroll(shortcutsRow),
	)
	if showTip {
		toolbar.Add(widget.NewLabelWithStyle("Tip: type a keyword (desktop, documents, downloads, home) into the path bar to jump directly.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
	}
	toolbar.Add(actionRow)

	scroll := container.NewVScroll(explorerBox)
	pickerWin.SetContent(container.NewBorder(toolbar, nil, nil, nil, scroll))
	refreshPicker()
	pickerWin.Show()
}

func showCustomFilePicker(parentWindow fyne.Window, title string, allowedExts []string, showTip bool, onSelect func(string)) {
	pickerWin := state.App.NewWindow(title)
	pickerWin.Resize(fyne.NewSize(550, 420))

	currentDir := state.SaveFolder
	if currentDir == "" {
		currentDir = "."
	}
	currentDir = filepath.Clean(currentDir)

	pathEntry := widget.NewEntry()
	pathEntry.SetText(currentDir)

	explorerBox := container.NewVBox()

	var refreshPicker func()
	refreshPicker = func() {
		if currentDir == "Drives" {
			if pathEntry != nil {
				pathEntry.SetText("My Computer (Drives)")
			}
			explorerBox.Objects = nil
			drives := getWindowsDrives()
			for _, dr := range drives {
				drName := dr
				iconWidget := widget.NewIcon(theme.FolderIcon())
				nameLabel := widget.NewLabel(drName)
				row := container.NewHBox(iconWidget, nameLabel, layout.NewSpacer())
				btn := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() {
					currentDir = drName
					refreshPicker()
				})
				row.Add(btn)
				explorerBox.Add(row)
			}
			explorerBox.Refresh()
			return
		}

		currentDir = filepath.Clean(currentDir)
		if pathEntry != nil {
			pathEntry.SetText(currentDir)
		}

		entries, err := os.ReadDir(currentDir)
		if err != nil {
			explorerBox.Objects = []fyne.CanvasObject{widget.NewLabel(fmt.Sprintf("Error reading dir: %v", err))}
			explorerBox.Refresh()
			return
		}

		var folders []os.DirEntry
		var files []os.DirEntry
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if entry.IsDir() {
				folders = append(folders, entry)
			} else {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if len(allowedExts) > 0 {
					allowed := false
					for _, aExt := range allowedExts {
						if aExt == ext {
							allowed = true
							break
						}
					}
					if !allowed {
						continue
					}
				}
				files = append(files, entry)
			}
		}

		allEntries := append(folders, files...)
		explorerBox.Objects = nil

		if len(allEntries) == 0 {
			explorerBox.Add(widget.NewLabel("No folders or supported files here."))
			explorerBox.Refresh()
			return
		}

		for _, entry := range allEntries {
			e := entry
			name := e.Name()
			isDir := e.IsDir()

			icon := theme.DocumentIcon()
			if isDir {
				icon = theme.FolderIcon()
			}

			iconWidget := widget.NewIcon(icon)
			displayName := name
			if len(displayName) > 40 {
				displayName = displayName[:37] + "..."
			}
			nameLabel := widget.NewLabel(displayName)

			row := container.NewHBox(
				iconWidget,
				nameLabel,
				layout.NewSpacer(),
			)

			btnText := "Open"
			if !isDir {
				btnText = "Select"
			}
			btn := widget.NewButtonWithIcon(btnText, icon, func() {
				if isDir {
					currentDir = filepath.Join(currentDir, name)
					refreshPicker()
				} else {
					fullPath := filepath.Join(currentDir, name)
					onSelect(fullPath)
					pickerWin.Close()
				}
			})
			row.Add(btn)
			explorerBox.Add(row)
		}
		explorerBox.Refresh()
	}

	pathEntry.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text == "My Computer (Drives)" || strings.ToLower(text) == "drives" {
			currentDir = "Drives"
			refreshPicker()
			return
		}
		// Semantic shortcut: typing a keyword like "desktop" / "documents"
		// jumps to the matching well-known folder without requiring the
		// full filesystem path.
		if match, ok := matchKnownFolder(text); ok {
			currentDir = match
			refreshPicker()
			return
		}
		cleanPath := filepath.Clean(text)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			currentDir = cleanPath
			refreshPicker()
		} else {
			dialog.ShowError(fmt.Errorf("Invalid directory or shortcut: %s\n\nTry a path, or a keyword like 'desktop', 'documents', 'downloads', 'home'.", text), pickerWin)
			pathEntry.SetText(currentDir)
		}
	}

	backBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if currentDir == "Drives" {
			return
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			if len(getWindowsDrives()) > 0 {
				currentDir = "Drives"
			}
			refreshPicker()
		} else {
			currentDir = parent
			refreshPicker()
		}
	})

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		refreshPicker()
	})

	pathRow := container.NewBorder(
		nil,
		nil,
		container.NewHBox(backBtn, refreshBtn),
		nil,
		pathEntry,
	)

	// Quick Jump shortcuts: one-click access to common destinations so users
	// don't have to navigate the whole filesystem to reach Desktop/Documents.
	shortcutsRow := container.NewHBox()
	for _, f := range resolveKnownFolders() {
		target := f.Path
		btn := widget.NewButtonWithIcon(f.Label, theme.FolderIcon(), func() {
			currentDir = target
			refreshPicker()
		})
		shortcutsRow.Add(btn)
	}
	if drives := getWindowsDrives(); len(drives) > 0 {
		btn := widget.NewButtonWithIcon("Drives", theme.ComputerIcon(), func() {
			currentDir = "Drives"
			refreshPicker()
		})
		shortcutsRow.Add(btn)
	}

	toolbar := container.NewVBox(
		pathRow,
		container.NewHScroll(shortcutsRow),
	)
	if showTip {
		toolbar.Add(widget.NewLabelWithStyle("Tip: type a keyword (desktop, documents, downloads, home) into the path bar to jump directly.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
	}

	scroll := container.NewVScroll(explorerBox)
	pickerWin.SetContent(container.NewBorder(toolbar, nil, nil, nil, scroll))
	refreshPicker()
	pickerWin.Show()
}

// -------------------------------------------------------------
// ONBOARDING SETUP WIZARD
// -------------------------------------------------------------

func showOnboardingWizard() {
	wizardWindow := state.App.NewWindow("LeGaJ - Setup Wizard")
	wizardWindow.Resize(fyne.NewSize(650, 520))

	// Navigation & Skip buttons
	nextBtn := widget.NewButton("Next", nil)
	backBtn := widget.NewButton("Back", nil)
	backBtn.Disable()
	nextBtn.Disable() // Disabled by default in step 1 until connections verified

	// The Skip button advances one step at a time (rather than closing the whole
	// wizard); its handler is assigned below once the step-navigation helpers exist.
	skipBtn := widget.NewButton("Skip Wizard", nil)

	// Step 1: Welcome & Paths & Connections Test & API Model Selection
	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetText(state.ApiKey)

	apiModelSelect := widget.NewSelect([]string{
		"gemini-3.1-flash-lite",
		"gemini-2.5-flash",
		"gemini-1.5-flash",
		"gemini-1.5-pro",
	}, func(selected string) {
		state.ApiModel = selected
	})
	if state.ApiModel == "" {
		state.ApiModel = "gemini-3.1-flash-lite"
	}
	apiModelSelect.SetSelected(state.ApiModel)

	resumePathLabel := widget.NewLabel("No resume file selected")
	resumePathLabel.Wrapping = fyne.TextWrapOff
	selectResumeBtn := widget.NewButtonWithIcon("Choose Resume File", theme.DocumentIcon(), func() {
		showCustomFilePicker(wizardWindow, "Select Resume File", []string{".pdf", ".docx", ".txt", ".md"}, true, func(path string) {
			resumePathLabel.SetText(path)
		})
	})

	saveFolderLabel := widget.NewLabel(state.SaveFolder)
	saveFolderLabel.Wrapping = fyne.TextWrapOff
	selectFolderBtn := widget.NewButtonWithIcon("Choose Save Folder", theme.FolderOpenIcon(), func() {
		showCustomFolderPicker(wizardWindow, "Choose Save Folder", true, func(picked string) {
			saveFolderLabel.SetText(picked)
		})
	})

	progressLabelConn := widget.NewLabel("Connections not verified yet.")
	testConnBtn := widget.NewButton("Test Connection & Directory", func() {
		progressLabelConn.SetText("Verifying Gemini API key & folder write permissions...")
		go func() {
			state.ApiKey = apiKeyEntry.Text
			state.ApiModel = apiModelSelect.Selected
			_, err := callGeminiGo(state.ApiKey, "Say Hi", false)
			apiOk := err == nil

			dirOk := false
			dummyFile := filepath.Join(saveFolderLabel.Text, ".legaj_test_write")
			err = os.WriteFile(dummyFile, []byte("test"), 0644)
			if err == nil {
				os.Remove(dummyFile)
				dirOk = true
			}

			fyne.Do(func() {
				if apiOk && dirOk {
					progressLabelConn.SetText("✓ API Connection and Folder permissions verified!")
					nextBtn.Enable()
				} else {
					errMsg := ""
					if !apiOk {
						errMsg += "Gemini API test failed. Check API key. "
					}
					if !dirOk {
						errMsg += "Folder is not writable. Check folder permissions."
					}
					progressLabelConn.SetText("✗ Verification failed: " + errMsg)
					nextBtn.Disable()
				}
			})
		}()
	})

	step1 := container.NewVBox(
		widget.NewLabelWithStyle("Welcome to LeGaJ!", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Step 1: Configure your API keys, model, resume, and output folder permissions."),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Gemini API Key"), apiKeyEntry,
			widget.NewLabel("Gemini API Model"), apiModelSelect,
			widget.NewLabel("Base Resume"), container.NewHBox(selectResumeBtn, resumePathLabel),
			widget.NewLabel("Save Folder"), container.NewHBox(selectFolderBtn, saveFolderLabel),
		),
		testConnBtn,
		progressLabelConn,
	)

	// Step 2: Preference & Parse
	roleEntry := widget.NewEntry()
	roleEntry.SetText("Product Manager")
	locPrefEntry := widget.NewEntry()
	locPrefEntry.SetText("New York, NY")
	salaryPrefEntry := widget.NewEntry()
	salaryPrefEntry.SetText("$100k-$120k")

	progressLabel2 := widget.NewLabel("")
	bypassBtn := widget.NewButton("Manually Enter Details (Bypass)", nil)
	bypassBtn.Hide()

	step2 := container.NewVBox(
		widget.NewLabelWithStyle("Step 2: Job Preferences & Resume Parsing", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Specify job search preferences. Gemini will parse and structure your resume details."),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Target Job Role"), roleEntry,
			widget.NewLabel("Location Prefs"), locPrefEntry,
			widget.NewLabel("Target Salary"), salaryPrefEntry,
		),
		progressLabel2,
		bypassBtn,
	)

	// Step 3: Verify Parsed Details
	nameEntry := widget.NewEntry()
	emailEntry := widget.NewEntry()
	phoneEntry := widget.NewEntry()
	locEntry := widget.NewEntry()
	linkedinEntry := widget.NewEntry()
	websiteEntry := widget.NewEntry()

	step3Form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Full Name"), nameEntry,
		widget.NewLabel("Email Address"), emailEntry,
		widget.NewLabel("Phone Number"), phoneEntry,
		widget.NewLabel("Location"), locEntry,
		widget.NewLabel("LinkedIn Link"), linkedinEntry,
		widget.NewLabel("Portfolio/Website"), websiteEntry,
	)

	step3 := container.NewVBox(
		widget.NewLabelWithStyle("Step 3: Verify Parsed Details", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Confirm or edit your personal info before verifying the search grounding:"),
		widget.NewSeparator(),
		step3Form,
	)

	// Step 4: Resume Tailoring Preferences (Added Step)
	const (
		strategyLabelNone     = "Base Résumé (No Tailoring)"
		strategyLabelIndustry = "Role-Targeted (Tailor Once)"
		strategyLabelJob      = "Per-Posting (ATS-Optimized) — Recommended"
	)
	tailoringStrategySelect := widget.NewSelect([]string{
		strategyLabelNone,
		strategyLabelIndustry,
		strategyLabelJob,
	}, func(selected string) {
		switch selected {
		case strategyLabelNone:
			state.TailoringStrategy = "none"
		case strategyLabelIndustry:
			state.TailoringStrategy = "industry"
		case strategyLabelJob:
			state.TailoringStrategy = "job"
		}
	})
	if state.TailoringStrategy == "" {
		state.TailoringStrategy = "job"
	}
	switch state.TailoringStrategy {
	case "none":
		tailoringStrategySelect.SetSelected(strategyLabelNone)
	case "industry":
		tailoringStrategySelect.SetSelected(strategyLabelIndustry)
	default:
		tailoringStrategySelect.SetSelected(strategyLabelJob)
	}

	step4 := container.NewVBox(
		widget.NewLabelWithStyle("Step 4: Resume Tailoring Preferences", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Choose how LeGaJ should tailor your résumé for applications:"),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Tailoring Strategy"), tailoringStrategySelect,
		),
		widget.NewLabel("Options explained:"),
		widget.NewLabel("- Base Résumé (No Tailoring): Submit your master résumé unchanged for every application."),
		widget.NewLabel("- Role-Targeted (Tailor Once): Optimize your résumé a single time for your primary target role, then reuse it."),
		widget.NewLabel("- Per-Posting (ATS-Optimized): Dynamically rewrite bullet points against each job description's keywords for maximum ATS match."),
	)

	// Step 5: Cover Letter Template Setup
	clChoiceSelect := widget.NewSelect([]string{
		"Use Default Generic Cover Letter Template (Recommended)",
		"Generify My Own Cover Letter",
	}, nil)
	clChoiceSelect.SetSelected("Use Default Generic Cover Letter Template (Recommended)")

	ownClEntry := widget.NewMultiLineEntry()
	ownClEntry.SetPlaceHolder("Paste your existing ideal cover letter here, or choose a file above. We will parse it for style/structure and generate a generic placeholder template...")
	ownClEntry.Disable() // Disabled by default
	ownClEntry.Hide()    // Hidden by default

	clStatusLabel := widget.NewLabel("")
	clStatusLabel.Wrapping = fyne.TextWrapWord

	var step5_cl *fyne.Container

	selectClFileBtn := widget.NewButtonWithIcon("Choose Cover Letter File (.docx, .pdf, .txt, .md)", theme.DocumentIcon(), func() {
		showCustomFilePicker(wizardWindow, "Select Cover Letter File", []string{".pdf", ".docx", ".txt", ".md"}, true, func(path string) {
			clStatusLabel.SetText("Reading file: " + filepath.Base(path) + "...")
			go func() {
				text, err := RunParseResume(path)
				fyne.Do(func() {
					if err != nil {
						clStatusLabel.SetText("Error reading file: " + err.Error())
					} else {
						ownClEntry.SetText(text)
						clStatusLabel.SetText("✓ Cover letter loaded from file. Review/edit the text below, then click 'Generify My Cover Letter'.")
					}
				})
			}()
		})
	})
	selectClFileBtn.Hide()

	generateClBtn := widget.NewButton("Generify My Cover Letter", func() {
		if apiKeyEntry.Text == "" {
			clStatusLabel.SetText("Error: Gemini API Key is missing. Please set it in Step 1.")
			return
		}
		if ownClEntry.Text == "" {
			clStatusLabel.SetText("Error: Please paste or choose a cover letter first.")
			return
		}
		clStatusLabel.SetText("Parsing style and genericizing cover letter via Gemini...")
		nextBtn.Disable()
		backBtn.Disable()

		go func() {
			_, err := generateCustomCoverLetterTemplate(apiKeyEntry.Text, ownClEntry.Text)
			fyne.Do(func() {
				if err != nil {
					clStatusLabel.SetText(fmt.Sprintf("Error genericizing cover letter: %v", err))
				} else {
					clStatusLabel.SetText("✓ Generic cover letter template successfully generated and compiled to PDF!")
				}
				nextBtn.Enable()
				backBtn.Enable()
			})
		}()
	})
	generateClBtn.Hide()

	ownClFormItem := container.NewVBox(
		widget.NewLabel("Paste or edit your cover letter:"),
		ownClEntry,
	)
	ownClFormItem.Hide()

	clChoiceSelect.OnChanged = func(selected string) {
		if selected == "Generify My Own Cover Letter" {
			ownClEntry.Enable()
			ownClEntry.Show()
			ownClFormItem.Show()
			selectClFileBtn.Show()
			generateClBtn.Show()
		} else {
			ownClEntry.Disable()
			ownClEntry.Hide()
			ownClFormItem.Hide()
			selectClFileBtn.Hide()
			generateClBtn.Hide()
			clStatusLabel.SetText("")
		}
		if step5_cl != nil {
			step5_cl.Refresh()
		}
	}

	step5_cl = container.NewVBox(
		widget.NewLabelWithStyle("Step 5: Cover Letter Template Setup", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Choose whether to use the default generic cover letter template, or select/paste your own cover letter to extract its style/structure:"),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Choice"), clChoiceSelect,
		),
		selectClFileBtn,
		ownClFormItem,
		generateClBtn,
		clStatusLabel,
	)

	// Step 6: Browser Job Clipper Setup
	wizardBookmarkletJs := generateBookmarkletJS()
	absExtensionPath, _ := filepath.Abs("extension")
	wizardClipperMd := "### 📎 Setup Browser Job Clipper\n" +
		"LeGaJ allows you to clip job listings directly from LinkedIn, Indeed, Greenhouse, Lever, Workday, Ashby, and iCIMS using browser tools.\n\n" +
		"#### Option A: Bookmarklet Tool (Quickest)\n" +
		"1. Click the **Copy Bookmarklet Code** button below.\n" +
		"2. Ensure your bookmarks bar is visible (`Ctrl+Shift+B` or `Cmd+Shift+B`).\n" +
		"3. Right-click the bookmarks bar, select **Add page** or **Add bookmark**.\n" +
		"4. Name it **Clip to LeGaJ** and paste the copied code into the **URL / Address** field.\n\n" +
		"---\n\n" +
		"#### Option B: Chrome/Edge Extension\n" +
		"1. Open `chrome://extensions` or `edge://extensions` in your browser.\n" +
		"2. Enable the **Developer mode** toggle (top-right).\n" +
		"3. Click **Load unpacked** and select the extension directory:\n" +
		"   `" + absExtensionPath + "`"

	wizardClipperRichText := widget.NewRichTextFromMarkdown(wizardClipperMd)
	wizardClipperRichText.Wrapping = fyne.TextWrapWord

	wizardCopyBtn := widget.NewButtonWithIcon("Copy Bookmarklet Code", theme.ContentCopyIcon(), func() {
		wizardWindow.Clipboard().SetContent(wizardBookmarkletJs)
		dialog.ShowInformation("Copied!", "Bookmarklet code copied to clipboard.", wizardWindow)
	})
	wizardCopyBtn.Importance = widget.HighImportance

	step6 := container.NewVBox(
		wizardClipperRichText,
		widget.NewSeparator(),
		container.NewHBox(wizardCopyBtn, layout.NewSpacer()),
	)

	stepContainer := container.NewMax(step1)
	currentStep := 1

	updateButtons := func() {
		if currentStep == 1 {
			backBtn.Disable()
			nextBtn.SetText("Next")
			// API & path must be validated first
			nextBtn.Disable()
			skipBtn.SetText("Skip Wizard")
		} else if currentStep == 2 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Parse Resume & Continue")
			skipBtn.SetText("Skip Wizard")
		} else if currentStep == 3 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Next")
			skipBtn.SetText("Skip Wizard")
		} else if currentStep == 4 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Next")
			skipBtn.SetText("Skip Wizard")
		} else if currentStep == 5 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Next")
			skipBtn.SetText("Skip Wizard")
		} else if currentStep == 6 {
			backBtn.Enable()
			nextBtn.Enable() // No verification gate; clipper setup is informational
			nextBtn.SetText("Finish Setup")
			skipBtn.SetText("Skip Step")
		}
	}

	// showStep switches the visible wizard panel to the given 1-based step.
	showStep := func(n int) {
		var panel fyne.CanvasObject
		// Skip past Step 4 (tailoring strategy) when the feature is disabled.
		if !resumeTailoringEnabled && n == 4 {
			n = 5
		}
		switch n {
		case 1:
			panel = step1
		case 2:
			panel = step2
		case 3:
			panel = step3
		case 4:
			panel = step4
		case 5:
			panel = step5_cl
		default:
			panel = step6
		}
		currentStep = n
		stepContainer.Objects = []fyne.CanvasObject{panel}
		stepContainer.Refresh()
		updateButtons()
	}

	// The Skip button advances a single step; on the final step it finishes setup.
	skipBtn.OnTapped = func() {
		nav := &wizardNavigator{currentStep: currentStep, totalSteps: 6}
		nextStep, shouldClose := nav.skip()
		if shouldClose {
			dialog.ShowInformation("Setup Finished", "Profile and connectivity setup complete. You can configure the browser clipper anytime from the Help tab.", wizardWindow)
			reloadAllViews()
			wizardWindow.Close()
			return
		}
		showStep(nextStep)
	}

	bypassAction := func() {
		currentStep = 3
		stepContainer.Objects = []fyne.CanvasObject{step3}
		stepContainer.Refresh()
		updateButtons()

		nameEntry.SetText("")
		emailEntry.SetText("")
		phoneEntry.SetText("")
		locEntry.SetText("")
		linkedinEntry.SetText("")
		websiteEntry.SetText("")
	}
	bypassBtn.OnTapped = bypassAction

	nextBtn.OnTapped = func() {
		if currentStep == 1 {
			state.ApiKey = apiKeyEntry.Text
			state.ApiModel = apiModelSelect.Selected
			state.SaveFolder = saveFolderLabel.Text
			saveConfigurations()

			currentStep = 2
			stepContainer.Objects = []fyne.CanvasObject{step2}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 2 {
			if roleEntry.Text == "" {
				dialog.ShowInformation("Required Info", "Please enter your target job role.", wizardWindow)
				return
			}

			progressLabel2.SetText("Reading resume and parsing structure via Gemini...")
			nextBtn.Disable()
			backBtn.Disable()
			bypassBtn.Show()

			go func() {
				outText, err := RunParseResume(resumePathLabel.Text)
				if err != nil {
					fyne.Do(func() {
						dialog.ShowError(err, wizardWindow)
						progressLabel2.SetText("Failed parsing file. You can bypass using the button below.")
						nextBtn.Enable()
						backBtn.Enable()
					})
					return
				}

				parsePrompt := fmt.Sprintf(`
You are an expert resume parsing AI. Extract the resume text below and convert it into a valid JSON object matching the following structure exactly. Do not add comments or additional text. Output ONLY valid JSON.

Structure:
{
  "personal_info": {
    "name": "Full Name",
    "email": "Email Address",
    "phone": "Phone Number",
    "location": "City, State",
    "linkedin": "linkedin URL (optional)",
    "website": "portfolio or personal website URL (optional)"
  },
  "target_roles": ["Role 1"],
  "education": [
    {
      "institution": "University",
      "degree": "Degree",
      "major": "Major",
      "graduation_date": "Date",
      "location": "Location",
      "gpa": "",
      "details": ""
    }
  ],
  "experience": [
    {
      "company": "Company",
      "role": "Role",
      "location": "Location",
      "start_date": "Start",
      "end_date": "End",
      "bullets": [
        "Bullet 1"
      ]
    }
  ],
  "projects": [],
  "skills": {
    "technical": ["Python"],
    "product_management": []
  }
}

Resume Text:
%s`, outText)

				parsedJsonStr, err := callGeminiGo(state.ApiKey, parsePrompt, true)
				if err != nil {
					fyne.Do(func() {
						dialog.ShowError(err, wizardWindow)
						progressLabel2.SetText("Failed calling Gemini API. You can bypass using the button below.")
						nextBtn.Enable()
						backBtn.Enable()
					})
					return
				}

				writeSecureFile("references/user-profile.json", []byte(parsedJsonStr))
				loadProfileData()

				fyne.Do(func() {
					nameEntry.SetText(state.Profile.PersonalInfo.Name)
					emailEntry.SetText(state.Profile.PersonalInfo.Email)
					phoneEntry.SetText(state.Profile.PersonalInfo.Phone)
					locEntry.SetText(state.Profile.PersonalInfo.Location)
					linkedinEntry.SetText(state.Profile.PersonalInfo.Linkedin)
					websiteEntry.SetText(state.Profile.PersonalInfo.Website)

					if state.Profile.PersonalInfo.Email == "" {
						emailEntry.SetPlaceHolder("Warning: Missing email address")
					}
					if state.Profile.PersonalInfo.Phone == "" {
						phoneEntry.SetPlaceHolder("Warning: Missing phone number")
					}

					currentStep = 3
					stepContainer.Objects = []fyne.CanvasObject{step3}
					stepContainer.Refresh()
					updateButtons()
					nextBtn.Enable()
					backBtn.Enable()
					bypassBtn.Hide()
				})
			}()
		} else if currentStep == 3 {
			state.Profile.PersonalInfo.Name = nameEntry.Text
			state.Profile.PersonalInfo.Email = emailEntry.Text
			state.Profile.PersonalInfo.Phone = phoneEntry.Text
			state.Profile.PersonalInfo.Location = locEntry.Text
			state.Profile.PersonalInfo.Linkedin = linkedinEntry.Text
			state.Profile.PersonalInfo.Website = websiteEntry.Text

			if len(state.Profile.TargetRoles) == 0 {
				state.Profile.TargetRoles = []string{roleEntry.Text}
			} else {
				state.Profile.TargetRoles[0] = roleEntry.Text
			}

			saveProfileData()

			// Step 4 (tailoring strategy) is hidden while resume tailoring is
			// disabled — jump straight to Step 5 (cover letter setup).
			if !resumeTailoringEnabled {
				state.TailoringStrategy = "none"
				saveConfigurations()
				currentStep = 5
				stepContainer.Objects = []fyne.CanvasObject{step5_cl}
				stepContainer.Refresh()
				updateButtons()
				return
			}

			currentStep = 4
			stepContainer.Objects = []fyne.CanvasObject{step4}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 4 {
			// Save tailoring strategy
			saveConfigurations()

			currentStep = 5
			stepContainer.Objects = []fyne.CanvasObject{step5_cl}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 5 {
			if clChoiceSelect.Selected == "Use Default Generic Cover Letter Template (Recommended)" {
				progressLabel := dialog.NewProgressInfinite("Compiling Cover Letter", "Writing default cover letter template and compiling PDF...", wizardWindow)
				progressLabel.Show()
				go func() {
					err := writeDefaultCoverLetterTemplate()
					fyne.Do(func() {
						progressLabel.Hide()
						if err != nil {
							dialog.ShowError(err, wizardWindow)
							return
						}
						currentStep = 6
						stepContainer.Objects = []fyne.CanvasObject{step6}
						stepContainer.Refresh()
						updateButtons()
					})
				}()
			} else {
				// Custom template was generated, proceed to step 6
				currentStep = 6
				stepContainer.Objects = []fyne.CanvasObject{step6}
				stepContainer.Refresh()
				updateButtons()
			}
		} else if currentStep == 6 {
			doneMsg := "Setup complete! You're ready to start tracking and tailoring applications."
			if !resumeTailoringEnabled {
				doneMsg = "Setup complete! You're ready to start tracking applications."
			}
			dialog.ShowInformation("Setup Finished", doneMsg, wizardWindow)
			reloadAllViews()
			wizardWindow.Close()
		}
	}

	backBtn.OnTapped = func() {
		if currentStep == 2 {
			currentStep = 1
			stepContainer.Objects = []fyne.CanvasObject{step1}
			stepContainer.Refresh()
			updateButtons()
			bypassBtn.Hide()
		} else if currentStep == 3 {
			currentStep = 2
			stepContainer.Objects = []fyne.CanvasObject{step2}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 4 {
			currentStep = 3
			stepContainer.Objects = []fyne.CanvasObject{step3}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 5 {
			currentStep = 4
			stepContainer.Objects = []fyne.CanvasObject{step4}
			stepContainer.Refresh()
			updateButtons()
		} else if currentStep == 6 {
			currentStep = 5
			stepContainer.Objects = []fyne.CanvasObject{step5_cl}
			stepContainer.Refresh()
			updateButtons()
		}

		// Back from step 5 lands on step 3 when tailoring is disabled
		// (step 4 is skipped in the forward direction).
		if !resumeTailoringEnabled && currentStep == 4 {
			currentStep = 3
			stepContainer.Objects = []fyne.CanvasObject{step3}
			stepContainer.Refresh()
			updateButtons()
		}
	}

	wizardWindow.SetContent(container.NewBorder(
		nil,
		container.NewHBox(backBtn, layout.NewSpacer(), skipBtn, layout.NewSpacer(), nextBtn),
		nil,
		nil,
		stepContainer,
	))
	wizardWindow.Show()
}

// -------------------------------------------------------------
// TRACK & TAILOR AUTOMATION PIPELINE
// -------------------------------------------------------------

func runTrackAndTailorAutomation(company, role, location, link, desc string) {
	// Strip portal brand suffixes
	company = titleSanitizerRegex.ReplaceAllString(company, "")
	company = strings.TrimSpace(company)
	role = titleSanitizerRegex.ReplaceAllString(role, "")
	role = strings.TrimSpace(role)

	// Resume tailoring is gated by resumeTailoringEnabled. When disabled, the
	// pipeline skips resume tailoring AND resume PDF compilation entirely —
	// the user's base resume is already what they'd attach, so generating a
	// duplicate copy serves no purpose. The pipeline still tracks the row and
	// drafts the cover letter.
	progressMsg := "Tracking row, drafting cover letter, compiling cover letter PDF..."
	tailoringStrategy := "none"
	if resumeTailoringEnabled {
		progressMsg = "Tracking row, tailoring resume, drafting cover letter, compiling PDFs..."
		tailoringStrategy = state.TailoringStrategy
	}
	progress := dialog.NewProgressInfinite("Pipeline Automating", progressMsg, state.Window)
	progress.Show()

	go func() {
		// Redundancy Check
		existingApp := findApplicationByLink(link)
		if existingApp == nil {
			existingApp = findApplicationByCompanyAndRole(company, role)
		}
		if existingApp != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowInformation("Already Tracked", fmt.Sprintf("You are already tracking '%s' at '%s' (Status: %s).", role, company, existingApp.Status), state.Window)
			})
			return
		}

		// 1. Add to excel tracker
		resumeNamePattern := "%s_Resume_Tailored.pdf"
		notesText := "Auto-tailored and tracked."
		if !resumeTailoringEnabled {
			// Reference the user's base resume file name directly so the
			// tracker row still points at something openable, even though
			// the pipeline does not compile a new resume PDF in this mode.
			resumeNamePattern = "%s_Resume.pdf"
			notesText = "Auto-tracked. Cover letter generated; attach your base resume."
		}
		resumePdfName := fmt.Sprintf(resumeNamePattern, strings.ReplaceAll(company, " ", "_"))
		coverLetterPdfName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(company, " ", "_"))

		if err := addApplicationGo(company, role, location, link, "Applied", resumePdfName, coverLetterPdfName, notesText); err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		// Load the base profile once for both the (optional) tailoring step
		// and the cover-letter prompt below.
		baseProfileBytes, err := os.ReadFile("references/user-profile.json")
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		// Steps 2 + 3 (tailor JSON, compile resume PDF) are skipped wholesale
		// when resume tailoring is disabled — generating a duplicate of the
		// base resume PDF on every clip produced clutter with no value.
		var resumeOutputPath string
		if resumeTailoringEnabled {
			// 2. Tailor resume JSON
			if tailoringStrategy == "none" {
				// Copy base profile directly to the tailored slot.
				err = writeSecureFile("references/user-profile-tailored.json", baseProfileBytes)
				if err != nil {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(err, state.Window)
					})
					return
				}
			} else {
				var tailorPrompt string
				if tailoringStrategy == "industry" {
				targetRole := role
				if len(state.Profile.TargetRoles) > 0 && state.Profile.TargetRoles[0] != "" {
					targetRole = state.Profile.TargetRoles[0]
				}
				tailorPrompt = fmt.Sprintf(`You are an expert resume writer. Rewrite ONLY the applicant's experience bullet points in the base profile JSON to align with the target job role/industry as a whole. All other fields must be preserved verbatim.

Base Profile JSON:
%s

Target Job Role/Industry:
%s

Mandates:
1. Rewrite ONLY the experience bullet points. Use the STAR framework (Situation, Task, Action, Result) and begin each bullet with a strong active past-tense verb.
2. Naturally incorporate relevant keywords for the target job role/industry into the rewritten bullets.
3. STRICTLY preserve every quantitative metric (percentages, dollar amounts, team sizes, time periods, dates). Never fabricate, omit, or alter any number.
4. Preserve VERBATIM: personal_info, target_roles, education, projects, skills, and every entry under additional_sections (publications, certifications, awards, etc.). Do not add, remove, reorder, or rename any field. Do not invent skills the applicant did not list. Do not create new sections.
5. Output ONLY valid JSON with the EXACT same set of top-level and nested keys as the input. No explanations, no markdown blocks.`, string(baseProfileBytes), targetRole)
			} else { // "job" or default
				tailorPrompt = fmt.Sprintf(`You are an expert resume writer. Rewrite ONLY the applicant's experience bullet points in the base profile JSON to align with the target job description. All other fields must be preserved verbatim.

Base Profile JSON:
%s

Target Job Description:
%s

Mandates:
1. Rewrite ONLY the experience bullet points. Use the STAR framework (Situation, Task, Action, Result) and begin each bullet with a strong active past-tense verb.
2. Naturally incorporate relevant keywords from the target job description into the rewritten bullets.
3. STRICTLY preserve every quantitative metric (percentages, dollar amounts, team sizes, time periods, dates). Never fabricate, omit, or alter any number.
4. Preserve VERBATIM: personal_info, target_roles, education, projects, skills, and every entry under additional_sections (publications, certifications, awards, etc.). Do not add, remove, reorder, or rename any field. Do not invent skills the applicant did not list. Do not create new sections.
5. Output ONLY valid JSON with the EXACT same set of top-level and nested keys as the input. No explanations, no markdown blocks.`, string(baseProfileBytes), desc)
			}

			tailoredJson, err := callGeminiGo(state.ApiKey, tailorPrompt, true)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}

			tailoredBytes := []byte(tailoredJson)
			if verr := validateTailoredProfile(baseProfileBytes, tailoredBytes); verr != nil {
				fmt.Printf("Tailored profile guard tripped for %s @ %s (%v) — falling back to base profile.\n", role, company, verr)
				tailoredBytes = baseProfileBytes
			}
			// Write tailored profile
			err = writeSecureFile("references/user-profile-tailored.json", tailoredBytes)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}
		}

			// 3. Compile resume PDF to save folder
			resumeOutputPath = filepath.Join(state.SaveFolder, resumePdfName)
			_, err = RunGenerateResume("references/user-profile-tailored.json", resumeOutputPath)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}
		}

		// 4. Draft Cover Letter aligning with the style guide
		var prof Profile
		json.Unmarshal(baseProfileBytes, &prof)
		candName := prof.PersonalInfo.Name
		if candName == "" {
			candName = "[Full Name]"
		}

		genericTemplate, genErr := ensureGenericCoverLetter(state.ApiKey, baseProfileBytes)
		if genErr != nil {
			fmt.Println("Warning: Could not load or generate generic cover letter template:", genErr)
		}

		coverPrompt := fmt.Sprintf(`Write a professional 4-paragraph cover letter for %s for the role of "%s" at "%s".

Important: Base the tone, style, and structure on the following generic template:
---
%s
---

Structure the cover letter exactly as follows:
Paragraph 1: State the position applied for ("%s" at "%s") and a hook showing alignment with company mission based on the provided job description:
"%s"
Paragraph 2-3: Map 2 specific, quantified achievements from the provided profile to the target requirements.
Paragraph 4: Professional closing and call to action.

Use the following profile details: %s

Strict Mandates:
1. Do NOT mention where the job listing was found or reference referral sources (such as LinkedIn, Indeed, etc.).
2. At the end of the letter, output only the sign-off 'Sincerely,' followed by the applicant's name. Do NOT output any other details (such as address, phone number, email, date, etc.) below the name or signature.

Output ONLY the cover letter text, no conversational intro or outro.`, candName, stripRoleMetadata(role), company, genericTemplate, stripRoleMetadata(role), company, desc, string(baseProfileBytes))

		coverLetterDraftText, err := callGeminiGo(state.ApiKey, coverPrompt, false)
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		// Save draft cover letter to temp file
		tempDraftPath := filepath.Join(os.TempDir(), fmt.Sprintf("legaj_auto_draft_%d.txt", time.Now().UnixNano()))
		err = os.WriteFile(tempDraftPath, []byte(coverLetterDraftText), 0644)
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}
		defer os.Remove(tempDraftPath)

		// 5. Compile Cover Letter PDF to save folder
		coverOutputPath := filepath.Join(state.SaveFolder, coverLetterPdfName)
		_, err = RunGenerateCoverLetter("references/user-profile.json", tempDraftPath, coverOutputPath)
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		fyne.Do(func() {
			progress.Hide()
			var msg string
			if resumeTailoringEnabled {
				msg = fmt.Sprintf("Successfully tracked job, tailored resume, and compiled PDFs!\n\nSaved Resume: %s\nSaved Cover Letter: %s", resumeOutputPath, coverOutputPath)
			} else {
				msg = fmt.Sprintf("Successfully tracked job and drafted cover letter!\n\nSaved Cover Letter: %s\n\nAttach your base resume directly when applying.", coverOutputPath)
			}
			dialog.ShowInformation("Pipeline Complete", msg, state.Window)
			reloadAllViews()
		})
	}()
}

// -------------------------------------------------------------
// FILE MANAGER VIEW & NAVIGATION
// -------------------------------------------------------------
var (
	fmCurrentDir   string
	fmHistory      []string
	fmExplorerBox  *fyne.Container
	fmPreviewBox   *fyne.Container
	fmPathEntry    *widget.Entry
	fmGridViewMode = false
	fmSelectedFile string
)

func selectFileManagerFile(filename string) {
	fmSelectedFile = filepath.Join(fmCurrentDir, filename)
	info, err := os.Stat(fmSelectedFile)
	if err != nil {
		fmPreviewBox.Objects = []fyne.CanvasObject{widget.NewLabel(fmt.Sprintf("Error reading file: %v", err))}
		fmPreviewBox.Refresh()
		return
	}

	sizeStr := fmt.Sprintf("%.2f KB", float64(info.Size())/1024.0)
	modTimeStr := info.ModTime().Format("2006-01-02 15:04:05")

	titleLabel := widget.NewLabelWithStyle(filename, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sizeLabel := widget.NewLabel(fmt.Sprintf("Size: %s", sizeStr))
	dateLabel := widget.NewLabel(fmt.Sprintf("Modified: %s", modTimeStr))

	openBtn := widget.NewButtonWithIcon("Open Document", theme.HelpIcon(), func() {
		openLink("file:///" + filepath.ToSlash(fmSelectedFile))
	})

	renameBtn := widget.NewButtonWithIcon("Rename", theme.DocumentCreateIcon(), func() {
		renameEntry := widget.NewEntry()
		renameEntry.SetText(filename)
		dialog.ShowCustomConfirm("Rename File", "Save", "Cancel", renameEntry, func(confirmed bool) {
			if confirmed && renameEntry.Text != "" {
				newPath := filepath.Join(fmCurrentDir, renameEntry.Text)
				err := os.Rename(fmSelectedFile, newPath)
				if err != nil {
					dialog.ShowError(err, state.Window)
				} else {
					selectFileManagerFile(renameEntry.Text)
					refreshFileManager()
				}
			}
		}, state.Window)
	})

	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Confirm Delete", fmt.Sprintf("Are you sure you want to permanently delete %s?", filename), func(confirmed bool) {
			if confirmed {
				err := os.Remove(fmSelectedFile)
				if err != nil {
					dialog.ShowError(err, state.Window)
				} else {
					fmSelectedFile = ""
					fmPreviewBox.Objects = []fyne.CanvasObject{widget.NewLabel("No file selected")}
					fmPreviewBox.Refresh()
					refreshFileManager()
				}
			}
		}, state.Window)
	})
	deleteBtn.Importance = widget.DangerImportance

	actionButtons := container.NewVBox(openBtn, renameBtn, deleteBtn)

	fmPreviewBox.Objects = []fyne.CanvasObject{
		container.NewVBox(
			titleLabel,
			widget.NewSeparator(),
			sizeLabel,
			dateLabel,
			widget.NewSeparator(),
			actionButtons,
		),
	}
	fmPreviewBox.Refresh()
}

func refreshFileManager() {
	if fmCurrentDir == "" {
		fmCurrentDir = state.SaveFolder
		if fmCurrentDir == "" {
			fmCurrentDir = "."
		}
	}
	if fmCurrentDir == "Drives" {
		if fmPathEntry != nil {
			fmPathEntry.SetText("My Computer (Drives)")
		}
		fmExplorerBox.Objects = nil
		drives := getWindowsDrives()
		for _, dr := range drives {
			drName := dr
			iconWidget := widget.NewIcon(theme.FolderIcon())
			nameLabel := widget.NewLabel(drName)
			row := container.NewHBox(iconWidget, nameLabel, layout.NewSpacer())
			btn := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() {
				fmCurrentDir = drName
				refreshFileManager()
			})
			row.Add(btn)
			fmExplorerBox.Add(row)
		}
		fmExplorerBox.Refresh()
		return
	}

	fmCurrentDir = filepath.Clean(fmCurrentDir)
	if fmPathEntry != nil {
		fmPathEntry.SetText(fmCurrentDir)
	}

	entries, err := os.ReadDir(fmCurrentDir)
	if err != nil {
		fmExplorerBox.Objects = []fyne.CanvasObject{widget.NewLabel(fmt.Sprintf("Error reading dir: %v", err))}
		fmExplorerBox.Refresh()
		return
	}

	var folders []os.DirEntry
	var files []os.DirEntry
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			folders = append(folders, entry)
		} else {
			files = append(files, entry)
		}
	}

	allEntries := append(folders, files...)

	if len(allEntries) == 0 {
		fmExplorerBox.Objects = []fyne.CanvasObject{widget.NewLabel("Directory is empty")}
		fmExplorerBox.Refresh()
		return
	}

	fmExplorerBox.Objects = nil
	for _, entry := range allEntries {
		e := entry
		name := e.Name()
		isDir := e.IsDir()

		icon := theme.DocumentIcon()
		if isDir {
			icon = theme.FolderIcon()
		}

		iconWidget := widget.NewIcon(icon)
		displayName := name
		if len(displayName) > 40 {
			displayName = displayName[:37] + "..."
		}
		nameLabel := widget.NewLabel(displayName)

		row := container.NewHBox(
			iconWidget,
			nameLabel,
			layout.NewSpacer(),
		)

		btnText := "Open"
		if !isDir {
			btnText = "View"
		}
		btn := widget.NewButtonWithIcon(btnText, icon, func() {
			if isDir {
				fmHistory = append(fmHistory, fmCurrentDir)
				fmCurrentDir = filepath.Join(fmCurrentDir, name)
				refreshFileManager()
			} else {
				selectFileManagerFile(name)
			}
		})
		row.Add(btn)

		// Add an 8px transparent spacer to prevent clipping the adjustable split bar
		fmSpacer := canvas.NewRectangle(color.Transparent)
		fmSpacer.SetMinSize(fyne.NewSize(8, 0))
		row.Add(fmSpacer)

		fmExplorerBox.Add(row)
	}
	fmExplorerBox.Refresh()
}

func buildFileManagerTab() fyne.CanvasObject {
	fmCurrentDir = state.SaveFolder
	if fmCurrentDir == "" {
		fmCurrentDir = "."
	}

	fmPathEntry = widget.NewEntry()
	fmPathEntry.SetText(fmCurrentDir)
	fmPathEntry.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text == "My Computer (Drives)" || strings.ToLower(text) == "drives" {
			fmCurrentDir = "Drives"
			refreshFileManager()
			return
		}
		if match, ok := matchKnownFolder(text); ok {
			fmCurrentDir = match
			refreshFileManager()
			return
		}
		cleanPath := filepath.Clean(text)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			fmCurrentDir = cleanPath
			refreshFileManager()
		} else {
			dialog.ShowError(fmt.Errorf("Invalid directory or shortcut: %s\n\nTry a path, or a keyword like 'desktop', 'documents', 'downloads', 'home'.", text), state.Window)
			fmPathEntry.SetText(fmCurrentDir)
		}
	}

	backBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if fmCurrentDir == "Drives" {
			return
		}
		parent := filepath.Dir(fmCurrentDir)
		if parent == fmCurrentDir {
			if len(getWindowsDrives()) > 0 {
				fmCurrentDir = "Drives"
			}
			refreshFileManager()
		} else {
			fmCurrentDir = parent
			refreshFileManager()
		}
	})

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		refreshFileManager()
	})

	pathRow := container.NewBorder(
		nil,
		nil,
		container.NewHBox(backBtn, refreshBtn),
		nil,
		fmPathEntry,
	)

	// Quick Jump shortcuts mirror the file/folder pickers so users don't have
	// to type filesystem paths to reach Desktop/Documents/Downloads/etc.
	shortcutsRow := container.NewHBox()
	for _, f := range resolveKnownFolders() {
		target := f.Path
		btn := widget.NewButtonWithIcon(f.Label, theme.FolderIcon(), func() {
			fmCurrentDir = target
			refreshFileManager()
		})
		shortcutsRow.Add(btn)
	}
	if drives := getWindowsDrives(); len(drives) > 0 {
		btn := widget.NewButtonWithIcon("Drives", theme.ComputerIcon(), func() {
			fmCurrentDir = "Drives"
			refreshFileManager()
		})
		shortcutsRow.Add(btn)
	}

	// The path-bar keyword tip is intentionally omitted here — the Quick Jump
	// shortcut buttons above already cover Desktop/Documents/Downloads/etc., so
	// the standalone hint looked redundant in the File Manager. It is retained in
	// the file/folder pickers used by the first-run setup wizard.
	toolbar := container.NewVBox(
		pathRow,
		container.NewHScroll(shortcutsRow),
	)

	fmExplorerBox = container.NewVBox()
	explorerScroll := container.NewVScroll(fmExplorerBox)
	explorerScroll.SetMinSize(fyne.NewSize(300, 400))

	fmPreviewBox = container.NewVBox(widget.NewLabel("Select a file to preview"))
	previewScroll := container.NewVScroll(fmPreviewBox)
	previewScroll.SetMinSize(fyne.NewSize(200, 400))

	split := container.NewHSplit(explorerScroll, previewScroll)
	split.SetOffset(0.55)

	content := container.NewBorder(toolbar, nil, nil, nil, split)

	refreshFileManager()

	return content
}

// clipMux is a dedicated ServeMux for the clip server, preventing double-registration
// panics if startClipServer were ever called more than once.
var clipMux = http.NewServeMux()
var clipServerStarted bool

var titleSanitizerRegex = regexp.MustCompile(`(?i)\s*[-|•|\|]\s*(LinkedIn|Indeed|ZipRecruiter|Glassdoor|Google).*`)

type IPRateLimiter struct {
	mu  sync.Mutex
	ips map[string][]time.Time
}

var limiter = IPRateLimiter{
	ips: make(map[string][]time.Time),
}

func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	window := 10 * time.Second
	maxRequests := 5

	var validTimes []time.Time
	for _, t := range l.ips[ip] {
		if now.Sub(t) < window {
			validTimes = append(validTimes, t)
		}
	}

	if len(validTimes) >= maxRequests {
		l.ips[ip] = validTimes
		return false
	}

	validTimes = append(validTimes, now)
	l.ips[ip] = validTimes
	return true
}

func saveActivePort(port int) {
	os.MkdirAll("references", 0755)
	_ = os.WriteFile("references/.clip_port", []byte(fmt.Sprintf("%d", port)), 0644)
}

func extractCompanyNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "Web"
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}

	prefixes := []string{"www.", "careers.", "jobs.", "recruiting.", "app."}
	for _, pref := range prefixes {
		if strings.HasPrefix(host, pref) {
			host = strings.TrimPrefix(host, pref)
		}
	}

	parts := strings.Split(host, ".")
	if len(parts) > 0 && parts[0] != "" {
		return strings.Title(parts[0])
	}
	return "Web"
}

func writeExtensionFiles(port int, token string) {
	err := os.MkdirAll("extension", 0755)
	if err != nil {
		fmt.Printf("Warning: Failed to create extension directory: %v\n", err)
		return
	}

	manifest := `{
  "manifest_version": 3,
  "name": "LeGaJ Clipper",
  "version": "1.0.0",
  "description": "Clip job listings directly into LeGaJ",
  "permissions": [
    "activeTab",
    "scripting"
  ],
  "action": {
    "default_title": "Clip Job to LeGaJ"
  },
  "background": {
    "service_worker": "background.js"
  }
}`

	background := `chrome.action.onClicked.addListener((tab) => {
  chrome.scripting.executeScript({
    target: { tabId: tab.id },
    files: ['content.js']
  });
});`

	content := fmt.Sprintf(`(async function() {
  var h = window.location.hostname, p = window.location.href, c = '', r = '', l = '', d = '';
  try {
    if (h.includes('linkedin.com')) {
      var t = document.title.replace(' | LinkedIn', '').split(' at ');
      c = (document.querySelector('[data-tracking-control-name="public_jobs_topcard-org-name"]') || document.querySelector('.topcard__org-name-link') || document.querySelector('.job-details-jobs-unified-top-card__company-name a') || { innerText: t[1] || '' }).innerText;
      r = (document.querySelector('h1.t-24') || document.querySelector('.topcard__title') || document.querySelector('.job-details-jobs-unified-top-card__job-title h1') || { innerText: t[0] || '' }).innerText;
      l = (document.querySelector('.topcard__flavor--bullet') || document.querySelector('.job-details-jobs-unified-top-card__bullet') || { innerText: '' }).innerText;
      d = (document.querySelector('.description__text') || document.querySelector('#job-details') || { innerText: '' }).innerText;
    } else if (h.includes('indeed.com')) {
      var t2 = document.title.replace(' - Indeed', '').split(' at ');
      r = (document.querySelector('[data-testid="jobsearch-JobInfoHeader-title"]') || document.querySelector('h1.jobsearch-JobInfoHeader-title') || { innerText: t2[0] || '' }).innerText;
      c = (document.querySelector('[data-company-name]') || document.querySelector('[data-testid="inlineHeader-companyName"]') || { innerText: t2[1] || '' }).innerText;
      l = (document.querySelector('[data-testid="inlineHeader-companyLocation"]') || { innerText: '' }).innerText;
      d = (document.querySelector('#jobDescriptionText') || { innerText: '' }).innerText;
    } else if (h.includes('greenhouse.io') || h.includes('grnh.se')) {
      var t3 = document.title.split(' - ');
      r = (document.querySelector('h1.app-title') || document.querySelector('.app__title h1') || { innerText: t3[0] || '' }).innerText;
      c = (document.querySelector('.company-name') || { innerText: t3[t3.length-1] || '' }).innerText;
      l = (document.querySelector('.location') || { innerText: '' }).innerText;
      d = (document.querySelector('#content') || { innerText: '' }).innerText;
    } else if (h.includes('lever.co')) {
      var t4 = document.title.split('|');
      r = (document.querySelector('h2') || { innerText: t4[0] || '' }).innerText;
      c = t4.length > 1 ? t4[t4.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.sort-by-time.posting-category') || { innerText: '' }).innerText;
      d = (document.querySelector('.section-wrapper') || { innerText: '' }).innerText;
    } else if (h.includes('myworkdayjobs.com') || h.includes('workday.com')) {
      var t5 = document.title.split('|');
      r = (document.querySelector('[data-automation-id="jobPostingHeader"]') || { innerText: t5[0] || '' }).innerText;
      c = t5.length > 1 ? t5[t5.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('[data-automation-id="locations"]') || { innerText: '' }).innerText;
      d = (document.querySelector('[data-automation-id="jobPostingDescription"]') || { innerText: '' }).innerText;
    } else if (h.includes('ashbyhq.com')) {
      var t6 = document.title.split(' at ');
      r = (document.querySelector('h1') || { innerText: t6[0] || '' }).innerText;
      c = t6.length > 1 ? t6[t6.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.ashby-job-posting-brief-location') || { innerText: '' }).innerText;
      d = (document.querySelector('.ashby-job-posting-description') || { innerText: '' }).innerText;
    } else if (h.includes('icims.com')) {
      var t7 = document.title.split('-');
      r = (document.querySelector('h1') || { innerText: t7[0] || '' }).innerText;
      c = t7.length > 1 ? t7[t7.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.iCIMS_InfoMsg') || { innerText: '' }).innerText;
      d = (document.querySelector('#jobDetails') || { innerText: '' }).innerText;
    }

    if (!r) {
      var sel = ['.job-title', '.posting-title', '.title', '.role', 'h1'];
      for (var i = 0; i < sel.length; i++) {
        var el = document.querySelector(sel[i]);
        if (el && el.innerText.trim()) { r = el.innerText.trim(); break; }
      }
      if (!r) {
        var og = document.querySelector('meta[property="og:title"]');
        if (og && og.content.trim()) r = og.content.trim();
      }
      if (!r && document.title) {
        var ct = document.title.replace(/\s*[-|•|\|]\s*(LinkedIn|Indeed|ZipRecruiter|Glassdoor|Google).*$/i, '').trim();
        r = ct;
      }
      if (!r || r.toLowerCase().includes('apply now') || r.toLowerCase().includes('current openings') || r.toLowerCase().includes('career page') || r.toLowerCase().includes('job details')) {
        var segs = window.location.pathname.split('/').filter(Boolean);
        for (var j = 0; j < segs.length; j++) {
          var s = segs[j];
          if (s.length > 5 && !s.includes('.') && isNaN(s)) {
            r = decodeURIComponent(s).replace(/[-_]/g, ' ').replace(/\b\w/g, function(x) { return x.toUpperCase(); });
            break;
          }
        }
      }
    }

    if (!c) {
      var ht = document.title;
      c = ht.includes(' at ') ? ht.split(' at ').pop().split('|')[0].split('-')[0].trim() : '';
    }

    if (!l) {
      var els = document.querySelectorAll('*');
      for (var i = 0; i < els.length; i++) {
        var el = els[i];
        if (el.children.length === 0 && el.innerText.trim()) {
          var idc = (el.id + ' ' + el.className).toLowerCase();
          if (idc.includes('location') || idc.includes('workplace') || idc.includes('remote') || idc.includes('hybrid')) {
            var txt = el.innerText.trim();
            if (txt.length > 2 && txt.length < 100) { l = txt; break; }
          }
        }
      }
      if (!l) {
        var geoRegex = /^[A-Z][a-zA-Z\s.]+,\s*[A-Z]{2}(\s+[A-Z][a-zA-Z\s.]+)?$/;
        var tags = document.querySelectorAll('h2, h3, h4, p, span');
        for (var i = 0; i < tags.length; i++) {
          var txt = tags[i].innerText.trim();
          if (geoRegex.test(txt) || txt.toLowerCase() === 'remote' || txt.toLowerCase().includes('remote,') || txt.toLowerCase().includes('hybrid,')) {
            l = txt;
            break;
          }
        }
      }
    }

    if (!d) {
      var selD = ['main', 'article', '[class*="description"]', '[id*="description"]', '#job-details', '#content', '.section-wrapper', '[data-automation-id="jobPostingDescription"]', '.ashby-job-posting-description', '#jobDescriptionText'];
      for (var i = 0; i < selD.length; i++) {
        var el = document.querySelector(selD[i]);
        if (el && el.innerText.trim()) { d = el.innerText.trim(); break; }
      }
      if (!d) d = document.body.innerText;
    }
  } catch (e) {}

  c = (c || '').trim().substring(0, 100);
  r = (r || '').trim().substring(0, 150);
  l = (l || '').trim().substring(0, 100);
  d = (d || '').trim().substring(0, 50000);

  if (!c && !r) {
    alert("❌ Could not scrape job details from this page.");
    return;
  }

  try {
    let res = await fetch('http://127.0.0.1:%d/clip', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
        'X-LeGaJ-Token': '%s'
      },
      body: JSON.stringify({ company: c, role: r || document.title, location: l, link: p, description: d })
    });
    if (res.ok) {
      alert('✅ Clipped successfully via LeGaJ Extension!');
    } else {
      var msg = await res.text();
      alert('❌ Failed to send clip. Server returned ' + res.status + ':\n' + msg.trim());
    }
  } catch (err) {
    alert('❌ Failed to send clip. Is LeGaJ running?');
  }
})();`, port, token)

	_ = os.WriteFile("extension/manifest.json", []byte(manifest), 0644)
	_ = os.WriteFile("extension/background.js", []byte(background), 0644)
	_ = os.WriteFile("extension/content.js", []byte(content), 0644)
}

func initClipServer() {
	tokenFile := "references/.clip_token"
	if data, err := os.ReadFile(tokenFile); err == nil {
		if t := strings.TrimSpace(string(data)); len(t) == 32 {
			clipAuthToken = t
		}
	}
	if clipAuthToken == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("SECURITY: crypto/rand unavailable — cannot generate auth token: %v", err)
		}
		clipAuthToken = hex.EncodeToString(b)
		os.MkdirAll("references", 0755)
		_ = os.WriteFile(tokenFile, []byte(clipAuthToken), 0600)
	}

	for port := 8080; port < 8100; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			clipListener = ln
			activePort = port
			saveActivePort(port)
			writeExtensionFiles(port, clipAuthToken)
			return
		} else {
			fmt.Printf("Port %d already in use, trying next...\n", port)
		}
	}
	fmt.Println("Port 8080 already in use, clip server disabled")
}

func startClipServer() {
	if clipServerStarted {
		return
	}
	clipServerStarted = true

	clipMux.HandleFunc("/clip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-LeGaJ-Token")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Rate Limiting Check
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !limiter.Allow(ip) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		// Token Verification
		token := r.Header.Get("X-LeGaJ-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != clipAuthToken {
			http.Error(w, "Unauthorized: your bookmarklet token is outdated.\n\nIn LeGaJ, open the Help tab, re-copy the bookmarklet code, and reinstall it in your browser.", http.StatusUnauthorized)
			return
		}

		// Max Body Size Guard (100KB)
		r.Body = http.MaxBytesReader(w, r.Body, 100*1024)

		var payload struct {
			Company     string `json:"company"`
			Role        string `json:"role"`
			Location    string `json:"location"`
			Link        string `json:"link"`
			Description string `json:"description"`
		}

		if r.Method == "GET" {
			q := r.URL.Query()
			payload.Company = q.Get("company")
			payload.Role = q.Get("role")
			payload.Location = q.Get("location")
			payload.Link = q.Get("link")
			payload.Description = q.Get("description")
		} else if r.Method == "POST" {
			contentType := r.Header.Get("Content-Type")
			if strings.Contains(contentType, "application/json") {
				err := json.NewDecoder(r.Body).Decode(&payload)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				err := r.ParseForm()
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				payload.Company = r.FormValue("company")
				payload.Role = r.FormValue("role")
				payload.Location = r.FormValue("location")
				payload.Link = r.FormValue("link")
				payload.Description = r.FormValue("description")
			}
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Strip CR/control characters from scraped input before any processing
		// so stored values and text views render cleanly.
		payload.Company = cleanText(payload.Company)
		payload.Role = cleanText(payload.Role)
		payload.Location = cleanText(payload.Location)
		payload.Link = cleanText(payload.Link)
		payload.Description = cleanText(payload.Description)

		// Snapshot the raw scraped values so we can flag low-confidence extractions
		// for user review after all correction passes have run (Bugs 8 + 10).
		scrapedCompanyRaw := payload.Company
		scrapedRoleRaw := payload.Role

		// Layer 0: pull capitalized brand candidates from the first sentence
		// of the description. A majority of JDs name the employer in the
		// opening sentence ("About TD Bank...", "Acme is hiring..."), so this
		// is high-signal context for both the LLM call and the post-extraction
		// validation pass below.
		companyHints := extractCompanyHints(payload.Description)

		// Trust-the-scrape fast path: when the bookmarklet DOM selectors
		// produced a clean-looking company value (non-empty, not in the
		// generic chrome list, reasonable length, no URL/host shape), keep
		// it. This is the LinkedIn small-company common case: the
		// `.job-details-jobs-unified-top-card__company-name` selector
		// usually returns the correct employer, and the LLM's rewrite is
		// what introduces error. The LLM pass only runs when the scrape
		// failed or produced junk.
		runLLMExtraction := true
		if !isGenericCompany(payload.Company) {
			c := strings.TrimSpace(payload.Company)
			if len(c) >= 2 && len(c) <= 60 && !strings.ContainsAny(c, "/\\") {
				runLLMExtraction = false
			}
		}

		// Layer 1: LLM verification pass with a tightened prompt that includes
		// the hint list and explicit negative examples ("never return Jobs /
		// Careers / Apply / hostname / etc."). Non-empty results override the
		// scraped values; on failure we fall through to the per-field
		// heuristics below, so this never blocks a clip. Skipped entirely
		// when the scraped Company is already clean (trust-the-scrape).
		if runLLMExtraction {
			if llmCompany, llmRole, llmLocation, ok := extractJobDetailsLLM(state.ApiKey, payload.Company, payload.Role, payload.Location, payload.Description, companyHints); ok {
				if llmCompany != "" {
					payload.Company = llmCompany
				}
				if llmRole != "" {
					payload.Role = llmRole
				}
				if llmLocation != "" {
					payload.Location = llmLocation
				}
			}
		}

		// Layer 2: drop junk company values that survived the LLM pass
		// (page-chrome leaks like "Jobs" / "Careers" / hostnames). Treats
		// matches as empty so the per-field fallback chain reruns.
		if isGenericCompany(payload.Company) {
			payload.Company = ""
		}

		if payload.Company == "" {
			// Prefer a first-sentence hint when one is available — these are
			// usually the cleanest brand name in the entire payload.
			if len(companyHints) > 0 {
				payload.Company = companyHints[0]
			}
		}

		if payload.Company == "" {
			payload.Company = extractCompanyFromTitle(payload.Role)
			if payload.Company == "" && payload.Description != "" {
				corrected := correctMissingCompany(state.ApiKey, payload.Description, payload.Role)
				if corrected != "" {
					payload.Company = corrected
				}
			}
			// Layer 3: URL hostname stem as a last-ditch deterministic
			// signal. Skips ATS hosts (greenhouse.io / lever.co / etc.) so
			// only employer-owned career sites contribute. When an API key
			// is available, the stem is piped through correctMissingCompany
			// for a brand-name cleanup; otherwise titlecase fallback.
			if payload.Company == "" {
				if stem := extractCompanyFromHost(payload.Link); stem != "" {
					if state.ApiKey != "" && payload.Description != "" {
						if cleaned := correctMissingCompany(state.ApiKey, payload.Description, stem); cleaned != "" && !isGenericCompany(cleaned) {
							payload.Company = cleaned
						}
					}
					if payload.Company == "" {
						payload.Company = brandFromHostStem(stem)
					}
				}
			}
			if payload.Company == "" {
				payload.Company = "Unknown Company"
			}
		}
		payload.Company = titleSanitizerRegex.ReplaceAllString(payload.Company, "")
		payload.Company = strings.TrimSpace(payload.Company)
		if isGenericCompany(payload.Company) {
			payload.Company = "Unknown Company"
		}

		if payload.Role == "" {
			payload.Role = "(Role not detected — update in tracker)"
		}
		payload.Role = titleSanitizerRegex.ReplaceAllString(payload.Role, "")
		payload.Role = strings.TrimSpace(payload.Role)

		// Generic title filtering and correction
		if isGenericRole(payload.Role) {
			corrected := correctGenericRole(state.ApiKey, payload.Description, payload.Company)
			if corrected != "" {
				payload.Role = corrected
			}
		}

		// Flag low-confidence extractions for user review (Bugs 8 + 10). The
		// clip is still saved so it never blocks the user; the inbox surfaces
		// the review marker, and the failures log records what fell back.
		needsReview := false
		var reviewReasons []string
		if payload.Company == "Unknown Company" {
			needsReview = true
			reviewReasons = append(reviewReasons, "company unresolved")
		} else if len(payload.Description) >= 200 && !companyAppearsInContext(payload.Company, payload.Description+"\n"+payload.Role, companyHints) {
			// Layer 4: when the description is long enough to be authoritative
			// (>= 200 chars), the final company should be referenced somewhere
			// in the description body, the role/title, or echoed by the
			// first-sentence hints. If none of those hold, the extraction is
			// likely a page-chrome leak that slipped past the earlier filters
			// — flag for user review. Short descriptions are not validated
			// since absence of evidence isn't evidence of absence.
			needsReview = true
			reviewReasons = append(reviewReasons, "company not referenced in description")
		}
		if payload.Role == "(Role not detected — update in tracker)" || isGenericRole(payload.Role) {
			needsReview = true
			reviewReasons = append(reviewReasons, "role unresolved or generic")
		}
		reviewReason := strings.Join(reviewReasons, "; ")
		if needsReview {
			logClipFailure(scrapedCompanyRaw, scrapedRoleRaw, payload.Company, payload.Role, payload.Link, reviewReason)
		}

		// Save clipped job to local storage file references/clipped-jobs.json
		saveClippedJob(ClippedJob{
			Company:      payload.Company,
			Role:         payload.Role,
			Location:     payload.Location,
			Link:         payload.Link,
			Description:  payload.Description,
			NeedsReview:  needsReview,
			ReviewReason: reviewReason,
		})

		// Safety Check: Avoid Fyne GUI operations in non-GUI / headless runs
		if !isGUIMode || state.ClipInboxBox == nil {
			if r.Method == "GET" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Clipped successfully (headless mode)"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "clipped in headless mode"})
			return
		}

		fyne.Do(func() {
			if state.ClipInboxBox == nil {
				return
			}
			// addClippedJobToInboxUI self-heals the empty-state placeholder, so no
			// manual header rebuild is needed here.
			addClippedJobToInboxUI(ClippedJob{
				Company:      payload.Company,
				Role:         payload.Role,
				Location:     payload.Location,
				Link:         payload.Link,
				Description:  payload.Description,
				NeedsReview:  needsReview,
				ReviewReason: reviewReason,
			})
			state.ClipInboxBox.Refresh()
		})

		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>LeGaJ Clipper</title>
    <style>
      body { font-family: system-ui, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #0f172a; color: #f8fafc; }
      .card { background: #1e293b; padding: 2.5rem; border-radius: 16px; box-shadow: 0 10px 15px -3px rgba(0,0,0,0.3); text-align: center; max-width: 400px; width: 90%; }
      h1 { color: #10b981; margin-top: 0; font-size: 1.8rem; }
      p { color: #94a3b8; line-height: 1.5; margin-bottom: 2rem; }
      button { background: #3b82f6; color: white; border: none; padding: 0.75rem 1.5rem; border-radius: 8px; cursor: pointer; font-size: 1rem; font-weight: 500; transition: background 0.2s; }
      button:hover { background: #2563eb; }
    </style>
</head>
<body>
    <div class="card">
      <h1>✓ Clipped Successfully!</h1>
      <p>Job application details for <strong>` + html.EscapeString(payload.Company) + `</strong> have been sent to your LeGaJ Job Leads inbox.</p>
      <button onclick="window.close()">Close Window</button>
    </div>
    <script>
      setTimeout(function() {
        window.close();
      }, 1500);
    </script>
</body>
</html>`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	if !isGUIMode || clipListener == nil {
		return
	}

	go func() {
		_ = http.Serve(clipListener, clipMux)
	}()
}
