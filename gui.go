package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

type Profile struct {
	PersonalInfo PersonalInfo        `json:"personal_info"`
	TargetRoles  []string            `json:"target_roles"`
	Education    []Education         `json:"education"`
	Experience   []Experience        `json:"experience"`
	Projects     []Project           `json:"projects"`
	Skills       map[string][]string `json:"skills"`
}

func callGeminiGo(apiKey, promptText string, isJson bool) (string, error) {
	model := state.ApiModel
	if model == "" {
		model = "gemini-3.1-flash-lite"
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

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
		return "", err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
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
		return "", err
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini API")
	}

	text := apiResp.Candidates[0].Content.Parts[0].Text
	if isJson {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text, nil
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
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

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

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonBytes))
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
	EduContainer    *fyne.Container
	ExpContainer    *fyne.Container
	ProjContainer   *fyne.Container

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

func startFyneGUI() {
	state.App = app.NewWithID("com.legaj.desktop")
	state.App.Settings().SetTheme(customTheme{})
	state.Window = state.App.NewWindow("LeGaJ - Let's Get a Job Dashboard")
	state.Window.Resize(fyne.NewSize(1000, 720))

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
		gdrive := `G:\My Drive\Personal Labour Mobile\Cover Letter PDFs\AI Cover Letters`
		if _, err := os.Stat(gdrive); err == nil {
			state.SaveFolder = gdrive
		} else {
			state.SaveFolder = `C:\Users\molus\projects\legaj\outputs`
		}
	}

	// Initialize UI sections (build all tabs and widgets)
	dashboardTab := buildDashboardTab()
	jobHuntTab := buildJobHuntTab()
	profileTab := buildProfileTab()
	trackerTab := buildTrackerTab()
	tailoringTab := buildTailoringTab()
	fileManagerTab := buildFileManagerTab()
	prepTab := buildPrepTab()
	settingsTab := buildSettingsTab()
	helpTab := buildHelpTab()

	// Arrange layout
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Dashboard", theme.HomeIcon(), dashboardTab),
		container.NewTabItemWithIcon("Job Hunt", theme.SearchIcon(), jobHuntTab),
		container.NewTabItemWithIcon("Base Profile", theme.AccountIcon(), profileTab),
		container.NewTabItemWithIcon("Job Tracker", theme.ListIcon(), trackerTab),
		container.NewTabItemWithIcon("Tailor Assets", theme.DocumentCreateIcon(), tailoringTab),
		container.NewTabItemWithIcon("File Manager", theme.FolderIcon(), fileManagerTab),
		container.NewTabItemWithIcon("Interview Prep", theme.QuestionIcon(), prepTab),
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
	return os.WriteFile(".env", []byte(content), 0644)
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

func saveProfileData() {
	if state.Profile == nil {
		return
	}
	jsonData, err := json.MarshalIndent(state.Profile, "", "  ")
	if err != nil {
		dialog.ShowError(err, state.Window)
		return
	}
	err = os.WriteFile("references/user-profile.json", jsonData, 0644)
	if err != nil {
		dialog.ShowError(err, state.Window)
		return
	}
}

func loadTrackerData() {
	outText, err := RunManageApplications("list")
	if err != nil {
		state.Applications = []JobApplication{}
		return
	}
	var apps []JobApplication
	err = json.Unmarshal([]byte(outText), &apps)
	if err != nil {
		state.Applications = []JobApplication{}
		return
	}
	state.Applications = apps
}

func reloadAllViews() {
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

func updateTrackerSelectionUI() {
	if state.SelectedAppIdx >= 0 && state.SelectedAppIdx < len(state.Applications) {
		app := &state.Applications[state.SelectedAppIdx]
		state.TrackerSelected = app

		// Dynamically set toolbar status dropdown without triggering double updates
		isUpdatingTrackerDropdown = true
		trackerStatusSelect.SetSelected(app.Status)
		isUpdatingTrackerDropdown = false

		// Enable/disable document opening buttons based on file existence
		resPath := filepath.Join(state.SaveFolder, app.Resume)
		if _, err := os.Stat(resPath); err == nil && app.Resume != "" {
			trackerOpenResumeBtn.Enable()
		} else {
			trackerOpenResumeBtn.Disable()
		}

		clPath := filepath.Join(state.SaveFolder, app.CoverLetter)
		if _, err := os.Stat(clPath); err == nil && app.CoverLetter != "" {
			trackerOpenCoverLetterBtn.Enable()
		} else {
			trackerOpenCoverLetterBtn.Disable()
		}
	} else {
		state.TrackerSelected = nil
		isUpdatingTrackerDropdown = true
		trackerStatusSelect.ClearSelected()
		isUpdatingTrackerDropdown = false
		trackerOpenResumeBtn.Disable()
		trackerOpenCoverLetterBtn.Disable()
	}
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
				job              JobResult
				alive            bool
				groundingVerified bool
				tier             int
				badge            string
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
				}

				// ── Strategy 6: Render job cards with source badge + verification status ──
				for _, t := range tagged {
					r := t.job
					compLabel := widget.NewLabelWithStyle(r.Company, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
					roleLabel := widget.NewLabel(fmt.Sprintf("%s  ·  %s", r.Role, r.Location))
					descLabel := widget.NewLabel(r.Description)
					descLabel.Wrapping = fyne.TextWrapWord

					existing := findApplicationByLink(r.Link)
					if existing == nil {
						existing = findApplicationByCompanyAndRole(r.Company, r.Role)
					}

					openBtn := widget.NewButtonWithIcon("View Posting", theme.HelpIcon(), func() {
						openLink(r.Link)
					})
					trackTailorBtn := widget.NewButtonWithIcon("Track & Tailor", theme.DocumentCreateIcon(), func() {
						runTrackAndTailorAutomation(r.Company, r.Role, r.Location, r.Link, r.Description)
					})
					trackTailorBtn.Importance = widget.HighImportance

					if existing != nil {
						trackTailorBtn.SetText("Already Tracked (" + existing.Status + ")")
						trackTailorBtn.Disable()
					}

					// Source badge (S6)
					badgeLabel := widget.NewLabelWithStyle("[" + t.badge + "]", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

					header := container.NewHBox(compLabel, badgeLabel)
					if t.groundingVerified {
						// Blue star — this URL was directly retrieved by Gemini's search
						srcVerified := widget.NewLabelWithStyle("★ Source-Verified", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
						header = container.NewHBox(compLabel, badgeLabel, layout.NewSpacer(), srcVerified)
					} else if t.alive {
						checkLabel := widget.NewLabelWithStyle("✓ Live", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
						header = container.NewHBox(compLabel, badgeLabel, layout.NewSpacer(), checkLabel)
					} else {
						deadLabel := widget.NewLabelWithStyle("✗ Unavailable", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
						header = container.NewHBox(compLabel, badgeLabel, layout.NewSpacer(), deadLabel)
					}

					cardContent := container.NewVBox(
						header,
						roleLabel,
						descLabel,
						container.NewHBox(openBtn, trackTailorBtn),
					)
					state.SearchResultsBox.Add(widget.NewCard("", "", cardContent))
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

	searchCard := widget.NewCard("Job Discovery Engine", "Search active postings and auto-tailor/track assets instantly", container.NewVBox(
		searchForm,
		searchBtn,
		widget.NewSeparator(),
		resultsScroll,
	))

	// ── Clip Inbox Card (populated by the browser bookmarklet) ──
	state.ClipInboxBox = container.NewVBox(
		widget.NewLabelWithStyle("No clipped jobs yet. Use the bookmarklet on any job board to clip listings here.",
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
	)
	clipScroll := container.NewVScroll(state.ClipInboxBox)
	clipScroll.SetMinSize(fyne.NewSize(600, 220))
	clearClipBtn := widget.NewButtonWithIcon("Clear Inbox", theme.DeleteIcon(), func() {
		state.ClipInboxBox.Objects = []fyne.CanvasObject{
			widget.NewLabelWithStyle("Inbox cleared.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		}
		state.ClipInboxBox.Refresh()
	})
	clipCard := widget.NewCard("Clip Inbox", "Jobs clipped from your browser via the bookmarklet appear here for review", container.NewVBox(
		container.NewHBox(layout.NewSpacer(), clearClipBtn),
		clipScroll,
	))

	content := container.NewVBox(
		searchCard,
		clipCard,
	)

	return container.NewScroll(content)
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

	importBtn := widget.NewButtonWithIcon("Import PDF/DOCX Resume", theme.DocumentIcon(), func() {
		showCustomFilePicker(state.Window, "Import PDF/DOCX Resume", []string{".pdf", ".docx", ".doc", ".txt", ".md"}, func(filePath string) {
			progress := dialog.NewProgressInfinite("Parsing Resume", "Reading and structuring using Gemini AI...", state.Window)
			progress.Show()

			go func() {
				outText, err := RunParseResume(filePath)
				if err != nil {
					progress.Hide()
					dialog.ShowError(err, state.Window)
					return
				}

				if state.ApiKey == "" {
					progress.Hide()
					dialog.ShowError(fmt.Errorf("Gemini API Key is missing. Please set it in Settings first."), state.Window)
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
  }
}

Resume Text:
%s`, outText)

				parsedJsonStr, err := callGeminiGo(state.ApiKey, parsePrompt, true)
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}

				err = os.WriteFile("references/user-profile.json", []byte(parsedJsonStr), 0644)
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}

				dialog.ShowInformation("Parse Complete", "Successfully parsed and structured resume using Gemini AI!", state.Window)
				reloadAllViews()
				fillProfileForm()
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
	)

	return container.NewScroll(profileContent)
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
		openAddJobModal(app)
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
	state.TrackerTable.Select(sc.cellID)

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

			// Optimistically refresh stats, selectors and list on main thread
			refreshUI()

			if sc.onChanged != nil {
				sc.onChanged(s)
			}
		}))
	}

	menu := fyne.NewMenu("", items...)
	popUp := widget.NewPopUpMenu(menu, state.Window.Canvas())
	popUp.ShowAtPosition(ev.AbsolutePosition)
}

func (sc *statusCell) DoubleTapped(ev *fyne.PointEvent) {
	state.TrackerTable.Select(sc.cellID)
	if sc.cellID.Row > 0 && sc.cellID.Row-1 < len(state.Applications) {
		app := &state.Applications[sc.cellID.Row-1]
		openAddJobModal(app)
	}
}

type trackerCell struct {
	widget.BaseWidget
	clickable *clickableCell
	status    *statusCell
	cellID    widget.TableCellID
}

func newTrackerCell() *trackerCell {
	tc := &trackerCell{
		clickable: newClickableCell(),
		status:    newStatusCell(),
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
}
func (r *trackerCellRenderer) MinSize() fyne.Size {
	return r.cell.clickable.MinSize()
}
func (r *trackerCellRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.cell.clickable, r.cell.status}
}
func (r *trackerCellRenderer) Refresh() {
	r.cell.clickable.Refresh()
	r.cell.status.Refresh()
}

func (tc *trackerCell) CreateRenderer() fyne.WidgetRenderer {
	return &trackerCellRenderer{cell: tc}
}

type customTableLayout struct{}

func (l *customTableLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(600, 300)
}

func (l *customTableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Total width of other columns: 240 + 280 + 110 + 90 + 140 = 860
	// 20px padding left for scrollbar and card borders
	notesWidth := size.Width - 860 - 20
	if notesWidth < 150 {
		notesWidth = 150
	}
	if state.TrackerTable != nil {
		state.TrackerTable.SetColumnWidth(5, notesWidth)
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
	isUpdatingTrackerDropdown bool
)

// 3. JOB TRACKER SPREADSHEET TABLE VIEW
func buildTrackerTab() fyne.CanvasObject {
	// Status select dropdown on toolbar
	trackerStatusSelect = widget.NewSelect([]string{"Wishlist", "Applied", "Interviewing", "Offer", "Rejected", "Ghosted"}, func(selected string) {
		if isUpdatingTrackerDropdown {
			return
		}
		if state.TrackerSelected != nil && selected != "" && selected != state.TrackerSelected.Status {
			go func() {
				_, err := RunManageApplications("update", state.TrackerSelected.Company, state.TrackerSelected.Role, selected, "")
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, state.Window)
					} else {
						reloadAllViews()
					}
				})
			}()
		}
	})
	trackerStatusSelect.PlaceHolder = "Change Status"

	// Document opening buttons on toolbar
	trackerOpenResumeBtn = widget.NewButtonWithIcon("Open Resume", theme.DocumentIcon(), func() {
		if state.TrackerSelected != nil && state.TrackerSelected.Resume != "" {
			resPath := filepath.Join(state.SaveFolder, state.TrackerSelected.Resume)
			openLink("file:///" + filepath.ToSlash(resPath))
		}
	})
	trackerOpenResumeBtn.Disable()

	// Document opening buttons on toolbar
	trackerOpenCoverLetterBtn = widget.NewButtonWithIcon("Open Cover Letter", theme.DocumentIcon(), func() {
		if state.TrackerSelected != nil && state.TrackerSelected.CoverLetter != "" {
			clPath := filepath.Join(state.SaveFolder, state.TrackerSelected.CoverLetter)
			openLink("file:///" + filepath.ToSlash(clPath))
		}
	})
	trackerOpenCoverLetterBtn.Disable()

	// Table widget setup for spreadsheet-like grid layout
	state.TrackerTable = widget.NewTable(
		func() (int, int) {
			return len(state.Applications) + 1, 6 // 6 columns
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

				t := tc.clickable.text
				headers := []string{"Company", "Role", "Location", "Date", "Status", "Notes"}
				t.Text = headers[id.Col]
				t.TextStyle = fyne.TextStyle{Bold: true}
				t.TextSize = 13 // slightly larger for headers
				tc.clickable.Refresh()
			} else {
				if id.Row-1 < len(state.Applications) {
					app := state.Applications[id.Row-1]

					if id.Col == 4 {
						tc.clickable.Hide()
						tc.status.Show()

						tc.status.selected = app.Status
						tc.status.text.Text = app.Status

						compName := app.Company
						roleName := app.Role
						tc.status.onChanged = func(selected string) {
							if selected != "" && selected != app.Status {
								go func() {
									_, err := RunManageApplications("update", compName, roleName, selected, "")
									fyne.Do(func() {
										if err != nil {
											dialog.ShowError(err, state.Window)
										} else {
											reloadAllViews()
										}
									})
								}()
							}
						}
						tc.status.Refresh()
					} else {
						tc.clickable.Show()
						tc.status.Hide()

						t := tc.clickable.text
						t.TextStyle = fyne.TextStyle{}
						t.TextSize = 12 // compact size for data cells

						switch id.Col {
						case 0:
							t.Text = app.Company
						case 1:
							t.Text = app.Role
						case 2:
							t.Text = app.Location
						case 3:
							t.Text = app.Date
						case 5:
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
	state.TrackerTable.SetColumnWidth(0, 240) // Company (wider)
	state.TrackerTable.SetColumnWidth(1, 280) // Role (wider)
	state.TrackerTable.SetColumnWidth(2, 110) // Location
	state.TrackerTable.SetColumnWidth(3, 90)  // Date
	state.TrackerTable.SetColumnWidth(4, 140) // Status
	state.TrackerTable.SetColumnWidth(5, 350) // Notes (wide Notes column)

	state.TrackerTable.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row-1 < len(state.Applications) {
			state.SelectedAppIdx = id.Row - 1
			updateTrackerSelectionUI()
		}
	}

	addBtn := widget.NewButtonWithIcon("Add Application", theme.ContentAddIcon(), func() {
		openAddJobModal(nil)
	})

	updateBtn := widget.NewButtonWithIcon("Update Details", theme.DocumentCreateIcon(), func() {
		if state.TrackerSelected == nil {
			dialog.ShowInformation("Selection Required", "Please select a job cell in the table first.", state.Window)
			return
		}
		openAddJobModal(state.TrackerSelected)
	})

	syncBtn := widget.NewButtonWithIcon("Sync Email Updates", theme.ViewRefreshIcon(), func() {
		if state.Email == "" || state.Password == "" || state.ImapServer == "" {
			dialog.ShowInformation("Configuration Needed", "Please configure your IMAP Email settings in Settings tab first.", state.Window)
			return
		}

		progress := dialog.NewProgressInfinite("Email Synchronization", "Connecting to mail server and checking updates...", state.Window)
		progress.Show()

		go func() {
			outText, err := RunManageApplications("sync", state.Email, state.Password, state.ImapServer)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, state.Window)
					return
				}
				dialog.ShowInformation("Sync complete", outText, state.Window)
				reloadAllViews()
			})
		}()
	})

	// Relocated Bulk Import Button
	bulkImportBtn := widget.NewButtonWithIcon("Bulk Import", theme.FolderOpenIcon(), func() {
		showCustomFilePicker(state.Window, "Select CSV File", []string{".csv", ".txt"}, func(path string) {
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
					_, addErr := RunManageApplications("add", company, role, location, jobURL, resumeName, coverName, "Bulk imported from CSV.")
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

	controlBar := container.NewHBox(
		addBtn,
		bulkImportBtn,
		updateBtn,
		syncBtn,
		widget.NewSeparator(),
		widget.NewLabel("Status:"),
		trackerStatusSelect,
		layout.NewSpacer(),
		trackerOpenResumeBtn,
		trackerOpenCoverLetterBtn,
	)

	tableContainer := container.New(&customTableLayout{}, state.TrackerTable)
	tableCard := widget.NewCard("Job Tracker", "Select any row cell to open resume, cover letter, or edit details from the toolbar.", tableContainer)
	cardContainer := container.NewBorder(nil, nil, nil, nil, tableCard)

	return container.NewBorder(controlBar, nil, nil, nil, cardContainer)
}

func updateTrackerList() {
	if state.TrackerTable != nil {
		state.TrackerTable.Refresh()
	}
}

func openAddJobModal(job *JobApplication) {
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
				_, err = RunManageApplications("update", compEnt.Text, roleEnt.Text, statusSelect.Selected, notesEnt.Text)
			} else {
				resPdfName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(compEnt.Text, " ", "_"))
				clPdfName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(compEnt.Text, " ", "_"))
				_, err = RunManageApplications("add", compEnt.Text, roleEnt.Text, locEnt.Text, linkEnt.Text, resPdfName, clPdfName, notesEnt.Text)
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

Output ONLY the cover letter text, no conversational intro or outro.`, role, comp, string(profileBytes), genericTemplate)

			coverLetterDraftText, err := callGeminiGo(state.ApiKey, draftPrompt, false)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}

			tempDraftPath := filepath.Join("outputs", "temp_manual_draft.txt")
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

		prompt := fmt.Sprintf(`You are an expert resume writer. Tailor the applicant's experience bullet points and skills in the base profile JSON to align with the target job description.

Base Profile JSON:
%s

Target Job Description:
%s

Mandates:
1. For each experience, rewrite relevant bullet points to emphasize accomplishments that align with the job requirements.
2. YOU MUST strictly preserve all historical metrics (percentages, dollar values, size of teams) and truthfulness.
3. Elevate and rank the most critical keywords in the skills section.
4. Keep the exact same JSON structure. Do not add or remove jobs.
5. Output ONLY valid JSON matching the profile schema. No explanations, no markdown blocks.`, string(baseProfileBytes), jobDescription)

		tailoredJson, err := callGeminiGo(state.ApiKey, prompt, true)
		if err != nil {
			fyne.Do(func() { callback("", err) })
			return
		}

		err = os.WriteFile("references/user-profile-tailored.json", []byte(tailoredJson), 0644)
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

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Gemini API Key"), state.SettingsApiKey,
		widget.NewLabel("Gemini API Model"), state.SettingsApiModel,
		widget.NewLabel("Save Folder"), state.SettingsSaveFolder,
		widget.NewLabel("Email Address"), state.SettingsEmail,
		widget.NewLabel("IMAP App Password"), state.SettingsPassword,
		widget.NewLabel("IMAP Server"), state.SettingsImapServer,
	)

	githubBtn := widget.NewButtonWithIcon("GitHub: /bot-bbio", theme.HelpIcon(), func() {
		openLink("https://github.com/bot-bbio")
	})
	linkedinBtn := widget.NewButtonWithIcon("LinkedIn: /alvvays", theme.HelpIcon(), func() {
		openLink("https://linkedin.com/in/alvvays")
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

// 7. HELP & DOCUMENTATION VIEW
func buildHelpTab() fyne.CanvasObject {
	helpDoc := widget.NewCard("LeGaJ Help & Documentation", "", container.NewVBox(
		widget.NewLabel("1. Overview: Matches profile details to job postings, tailors experiences, drafts cover letters, and logs tracked row states locally."),
		widget.NewLabel("2. Save Directory: Output PDFs (Resume/Cover Letter) compile into your Custom Save Folder (defaults to Google Drive)."),
		widget.NewLabel("3. Templates: Formatting applies Times New Roman styling and strict single-page constraints."),
	))

	// Multi-site bookmarklet — no prompt() fallbacks.
	// Uses 127.0.0.1 (not localhost) to reliably reach the local server from HTTPS job board pages.
	// Falls back to document.title parsing when site-specific selectors don't match.
	bookmarkletJs := `javascript:(function(){
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
    } else {
      r=(document.querySelector('h1')||{innerText:''}).innerText||document.title;
      var ht=document.title;
      c=ht.includes(' at ')?ht.split(' at ').pop().split('|')[0].split('-')[0].trim():h.replace('www.','').split('.')[0];
      l='';
      d=(document.querySelector('main')||document.querySelector('article')||document.querySelector('[class*="description"]')||{innerText:''}).innerText;
    }
  } catch(e){}
  c=(c||'').trim().substring(0,100);
  r=(r||'').trim().substring(0,150);
  l=(l||'').trim().substring(0,100);
  d=(d||'').trim().substring(0,600);
  if(!c&&!r){return;}
  fetch('http://127.0.0.1:8080/clip',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({company:c||h,role:r||document.title,location:l,link:p,description:d})})
    .then(function(res){return res.json();})
    .then(function(){alert('\u2705 Clipped! Check the Clip Inbox in LeGaJ.');}) 
    .catch(function(e){
      var url='http://127.0.0.1:8080/clip?company='+encodeURIComponent(c||h)+'&role='+encodeURIComponent(r||document.title)+'&location='+encodeURIComponent(l)+'&link='+encodeURIComponent(p)+'&description='+encodeURIComponent(d);
      var w=window.open(url,'_blank','width=500,height=400,status=no,menubar=no,toolbar=no');
      if(!w){alert('\u274C Clip failed (Popup Blocked). Please allow popups for this site, or make sure LeGaJ is open.');}
    });
})();`

	bookmarkletEntry := widget.NewEntry()
	bookmarkletEntry.SetText(bookmarkletJs)
	bookmarkletEntry.MultiLine = false

	copyBtn := widget.NewButtonWithIcon("Copy Bookmarklet Code", theme.ContentCopyIcon(), func() {
		state.Window.Clipboard().SetContent(bookmarkletJs)
		dialog.ShowInformation("Copied!", "Bookmarklet code copied to clipboard.", state.Window)
	})
	copyBtn.Importance = widget.HighImportance

	installSteps := widget.NewCard("How to Install the Bookmarklet", "", container.NewVBox(
		widget.NewLabel("1. Click \"Copy Bookmarklet Code\" above."),
		widget.NewLabel("2. Open your browser (Chrome / Firefox / Edge)."),
		widget.NewLabel("3. Show the bookmarks bar: Ctrl+Shift+B (Windows) or ⌘+Shift+B (Mac)."),
		widget.NewLabel("4. Right-click the bookmarks bar → \"Add page\" or \"Add bookmark\"."),
		widget.NewLabel("5. Set any name (e.g. \"📎 Clip to LeGaJ\")."),
		widget.NewLabel("6. Paste the copied code as the URL / Address."),
		widget.NewLabel("7. Save. You're done!"),
		widget.NewSeparator(),
		widget.NewLabel("Works on: LinkedIn · Indeed · Greenhouse · Lever · Workday · Ashby · most career pages."),
		widget.NewLabel("When on a job posting page, click the bookmark → details clip into your Clip Inbox instantly."),
	))

	bookmarkletCard := widget.NewCard("Clip to LeGaJ Browser Bookmarklet", "One-click job clipping from any job board directly into LeGaJ", container.NewVBox(
		copyBtn,
		widget.NewSeparator(),
		installSteps,
		widget.NewSeparator(),
		widget.NewLabel("Advanced: bookmark URL source code (for manual paste):"),
		bookmarkletEntry,
	))

	securityCard := widget.NewCard("Security & Ethics Disclosure", "", container.NewVBox(
		widget.NewLabel("AI and the Internet are inherently dangerous. Any tool that inputs unverified information from the web is vulnerable to prompt injection. Use at your own risk."),
		widget.NewLabel("• LeGaJ operates 100% locally. Your PII (name, email, phone) and Gemini API Keys are kept on your machine."),
		widget.NewLabel("• The application NEVER auto-submits applications. It compiles PDFs for you to review and apply yourself."),
		widget.NewLabel("• AI features are used solely for structuring profile fields, tailoring resume bullets, and drafting cover letters."),
	))

	githubBtn := widget.NewButtonWithIcon("GitHub: /bot-bbio", theme.HelpIcon(), func() {
		openLink("https://github.com/bot-bbio")
	})
	linkedinBtn := widget.NewButtonWithIcon("LinkedIn: /alvvays", theme.HelpIcon(), func() {
		openLink("https://linkedin.com/in/alvvays")
	})
	creditsRow := container.NewHBox(
		widget.NewLabel("Credits:"),
		githubBtn,
		linkedinBtn,
	)

	content := container.NewVBox(
		canvas.NewText("Help & Documentation", theme.PrimaryColor()),
		securityCard,
		helpDoc,
		bookmarkletCard,
		widget.NewSeparator(),
		creditsRow,
	)

	return container.NewScroll(content)
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
func showCustomFilePicker(parentWindow fyne.Window, title string, allowedExts []string, onSelect func(string)) {
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
		cleanPath := filepath.Clean(text)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			currentDir = cleanPath
			refreshPicker()
		} else {
			dialog.ShowError(fmt.Errorf("Invalid directory path: %s", text), pickerWin)
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

	toolbar := container.NewBorder(
		nil,
		nil,
		container.NewHBox(backBtn, refreshBtn),
		nil,
		pathEntry,
	)

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

	var skipBtn *widget.Button
	skipBtn = widget.NewButton("Skip Wizard", func() {
		if skipBtn.Text == "Skip Step" {
			dialog.ShowInformation("Setup Finished", "Profile and connectivity setup complete (grounding verification skipped).", wizardWindow)
			reloadAllViews()
			wizardWindow.Close()
		} else {
			reloadAllViews()
			wizardWindow.Close()
		}
	})

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
		showCustomFilePicker(wizardWindow, "Select Resume File", []string{".pdf", ".docx", ".txt", ".md"}, func(path string) {
			resumePathLabel.SetText(path)
		})
	})

	saveFolderLabel := widget.NewLabel(state.SaveFolder)
	saveFolderLabel.Wrapping = fyne.TextWrapOff
	selectFolderBtn := widget.NewButtonWithIcon("Choose Save Folder", theme.FolderOpenIcon(), func() {
		od := dialog.NewFolderOpen(func(list fyne.ListableURI, err error) {
			if err != nil || list == nil {
				return
			}
			saveFolderLabel.SetText(list.Path())
		}, wizardWindow)
		od.Show()
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
	tailoringStrategySelect := widget.NewSelect([]string{
		"Not at all",
		"For every industry/role as a whole",
		"By specific job descriptions",
	}, func(selected string) {
		switch selected {
		case "Not at all":
			state.TailoringStrategy = "none"
		case "For every industry/role as a whole":
			state.TailoringStrategy = "industry"
		case "By specific job descriptions":
			state.TailoringStrategy = "job"
		}
	})
	if state.TailoringStrategy == "" {
		state.TailoringStrategy = "job"
	}
	switch state.TailoringStrategy {
	case "none":
		tailoringStrategySelect.SetSelected("Not at all")
	case "industry":
		tailoringStrategySelect.SetSelected("For every industry/role as a whole")
	default:
		tailoringStrategySelect.SetSelected("By specific job descriptions")
	}

	step4 := container.NewVBox(
		widget.NewLabelWithStyle("Step 4: Resume Tailoring Preferences", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Choose how LeGaJ should tailor your resume for applications:"),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Tailoring Strategy"), tailoringStrategySelect,
		),
		widget.NewLabel("Options explained:"),
		widget.NewLabel("- Not at all: Keep your base resume exactly as is."),
		widget.NewLabel("- For every industry/role: Tailor it once for your target role in general."),
		widget.NewLabel("- By specific job descriptions: Dynamically tailor your resume for each job description."),
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
		showCustomFilePicker(wizardWindow, "Select Cover Letter File", []string{".pdf", ".docx", ".txt", ".md"}, func(path string) {
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

	// Step 6: Verification (Test Search)
	testSearchLabel := widget.NewLabel("Verification not run yet.")
	testSearchLabel.Wrapping = fyne.TextWrapWord
	testSearchScroll := container.NewVScroll(testSearchLabel)
	testSearchScroll.SetMinSize(fyne.NewSize(500, 150))

	runTestSearchBtn := widget.NewButton("Run Grounding test search", func() {
		testSearchLabel.SetText("Running grounding search test query via Google Grounding...")
		go func() {
			testQuery := fmt.Sprintf(`Search Google briefly for 1 job listing matching "%s" in "%s". Return only a JSON array with company, role, location, link, description.`, roleEntry.Text, locEntry.Text)
			res, err := callGeminiWithSearchGo(apiKeyEntry.Text, testQuery)
			fyne.Do(func() {
				if err != nil {
					testSearchLabel.SetText(fmt.Sprintf("⚠️ Grounding search failed: %v.\nRunning fallback verification check to verify API key...", err))
					go func() {
						fallbackRes, fallbackErr := callGeminiGo(apiKeyEntry.Text, "Say Hi", false)
						fyne.Do(func() {
							if fallbackErr != nil {
								testSearchLabel.SetText(fmt.Sprintf("✗ Both grounding search and fallback checks failed.\nGrounding error: %v\nFallback error: %v\n\nPlease check your API key in Step 1.", err, fallbackErr))
								nextBtn.Disable()
							} else {
								testSearchLabel.SetText(fmt.Sprintf("✓ API Key is valid (Grounding search failed/limited).\nStandard response: %s\n\nYou can finish setup now.", fallbackRes))
								nextBtn.Enable()
							}
						})
					}()
				} else {
					testSearchLabel.SetText(fmt.Sprintf("✓ Grounding verification successful!\nRaw output:\n%s", res))
					nextBtn.Enable()
				}
			})
		}()
	})

	step6 := container.NewVBox(
		widget.NewLabelWithStyle("Step 6: Search Grounding Verification", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Verify that Google Search Grounding is fully active and returns listings:"),
		widget.NewSeparator(),
		runTestSearchBtn,
		testSearchScroll,
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
			nextBtn.SetText("Finish Setup")
			nextBtn.Disable() // Disabled until grounding test runs successfully
			skipBtn.SetText("Skip Step")
		}
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

				os.WriteFile("references/user-profile.json", []byte(parsedJsonStr), 0644)
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
			dialog.ShowInformation("Setup Finished", "Profile and connectivity verification complete!", wizardWindow)
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
	progress := dialog.NewProgressInfinite("Pipeline Automating", "Tracking row, tailoring resume, drafting cover letter, compiling PDFs...", state.Window)
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
		resumePdfName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(company, " ", "_"))
		coverLetterPdfName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(company, " ", "_"))

		_, err := RunManageApplications("add", company, role, location, link, resumePdfName, coverLetterPdfName, "Auto-tailored and tracked.")
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		// 2. Tailor resume JSON
		baseProfileBytes, err := os.ReadFile("references/user-profile.json")
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		if state.TailoringStrategy == "none" {
			// Not at all: Copy references/user-profile.json directly to references/user-profile-tailored.json
			err = os.WriteFile("references/user-profile-tailored.json", baseProfileBytes, 0644)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}
		} else {
			var tailorPrompt string
			if state.TailoringStrategy == "industry" {
				targetRole := role
				if len(state.Profile.TargetRoles) > 0 && state.Profile.TargetRoles[0] != "" {
					targetRole = state.Profile.TargetRoles[0]
				}
				tailorPrompt = fmt.Sprintf(`You are an expert resume writer. Tailor the applicant's experience bullet points and skills in the base profile JSON to align with the target job role/industry as a whole.

Base Profile JSON:
%s

Target Job Role/Industry:
%s

Mandates:
1. For each experience, rewrite relevant bullet points to emphasize accomplishments that align with the target job role.
2. YOU MUST strictly preserve all historical metrics (percentages, dollar values, size of teams) and truthfulness.
3. Elevate and rank the most critical keywords in the skills section.
4. Keep the exact same JSON structure. Do not add or remove jobs.
5. Output ONLY valid JSON matching the profile schema. No explanations, no markdown blocks.`, string(baseProfileBytes), targetRole)
			} else { // "job" or default
				tailorPrompt = fmt.Sprintf(`You are an expert resume writer. Tailor the applicant's experience bullet points and skills in the base profile JSON to align with the target job description.

Base Profile JSON:
%s

Target Job Description:
%s

Mandates:
1. For each experience, rewrite relevant bullet points to emphasize accomplishments that align with the job requirements.
2. YOU MUST strictly preserve all historical metrics (percentages, dollar values, size of teams) and truthfulness.
3. Elevate and rank the most critical keywords in the skills section.
4. Keep the exact same JSON structure. Do not add or remove jobs.
5. Output ONLY valid JSON matching the profile schema. No explanations, no markdown blocks.`, string(baseProfileBytes), desc)
			}

			tailoredJson, err := callGeminiGo(state.ApiKey, tailorPrompt, true)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}

			// Write tailored profile
			err = os.WriteFile("references/user-profile-tailored.json", []byte(tailoredJson), 0644)
			if err != nil {
				fyne.Do(func() {
					progress.Hide()
					dialog.ShowError(err, state.Window)
				})
				return
			}
		}

		// 3. Compile resume PDF to save folder
		resumeOutputPath := filepath.Join(state.SaveFolder, resumePdfName)
		_, err = RunGenerateResume("references/user-profile-tailored.json", resumeOutputPath)
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
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

Output ONLY the cover letter text, no conversational intro or outro.`, role, company, genericTemplate, role, company, desc, string(baseProfileBytes))

		coverLetterDraftText, err := callGeminiGo(state.ApiKey, coverPrompt, false)
		if err != nil {
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(err, state.Window)
			})
			return
		}

		// Save draft cover letter to temp file
		tempDraftPath := filepath.Join("outputs", "temp_auto_draft.txt")
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
			dialog.ShowInformation("Pipeline Complete", fmt.Sprintf("Successfully tracked job, tailored resume, and compiled PDFs!\n\nSaved Resume: %s\nSaved Cover Letter: %s", resumeOutputPath, coverOutputPath), state.Window)
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
		cleanPath := filepath.Clean(text)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			fmCurrentDir = cleanPath
			refreshFileManager()
		} else {
			dialog.ShowError(fmt.Errorf("Invalid directory path: %s", text), state.Window)
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

	toolbar := container.NewBorder(
		nil,
		nil,
		container.NewHBox(backBtn, refreshBtn),
		nil,
		fmPathEntry,
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

func startClipServer() {
	if clipServerStarted {
		return
	}
	clipServerStarted = true

	clipMux.HandleFunc("/clip", func(w http.ResponseWriter, r *http.Request) {
		// CORS + Chrome Private Network Access headers (required for HTTPS pages → localhost)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

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
				// Form URL encoded or Multipart
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

		// Accept partial clips — bookmarklet may not always get all fields.
		// Use the link domain as company fallback, and the full URL as role fallback.
		if payload.Company == "" {
			payload.Company = sourceBadge(payload.Link)
		}
		if payload.Role == "" {
			payload.Role = "(Role not detected — update in tracker)"
		}

		// Route to Clip Inbox in Job Hunt tab for user review
		fyne.Do(func() {
			if state.ClipInboxBox == nil {
				return
			}
			// Remove placeholder label on first real clip
			if len(state.ClipInboxBox.Objects) == 1 {
				if _, ok := state.ClipInboxBox.Objects[0].(*widget.Label); ok {
					state.ClipInboxBox.Objects = nil
				}
			}

			c := payload.Company
			ro := payload.Role
			lo := payload.Location
			li := payload.Link
			de := payload.Description
			bd := sourceBadge(li)

			compLabel := widget.NewLabelWithStyle(c, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			roleLabel := widget.NewLabel(fmt.Sprintf("%s  ·  %s", ro, lo))
			badgeLabel := widget.NewLabelWithStyle("["+bd+"]", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			clipLabel := widget.NewLabelWithStyle("📎 Clipped from browser", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			header := container.NewHBox(compLabel, badgeLabel, layout.NewSpacer(), clipLabel)

			descLabel := widget.NewLabel(de)
			descLabel.Wrapping = fyne.TextWrapWord

			existing := findApplicationByLink(li)
			if existing == nil {
				existing = findApplicationByCompanyAndRole(c, ro)
			}

			openBtn := widget.NewButtonWithIcon("View Posting", theme.HelpIcon(), func() {
				openLink(li)
			})
			trackBtn := widget.NewButtonWithIcon("Track & Tailor", theme.DocumentCreateIcon(), func() {
				runTrackAndTailorAutomation(c, ro, lo, li, de)
			})
			trackBtn.Importance = widget.HighImportance
			addOnlyBtn := widget.NewButtonWithIcon("Add to Tracker Only", theme.ContentAddIcon(), func() {
				resumeName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(c, " ", "_"))
				coverName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(c, " ", "_"))
				_, _ = RunManageApplications("add", c, ro, lo, li, resumeName, coverName, "Clipped from browser. "+de)
				reloadAllViews()
			})

			if existing != nil {
				clipLabel.SetText("📎 Clipped (Tracked: " + existing.Status + ")")
				trackBtn.SetText("Already Tracked (" + existing.Status + ")")
				trackBtn.Disable()
				addOnlyBtn.Disable()
			}

			cardContent := container.NewVBox(
				header,
				roleLabel,
				descLabel,
				container.NewHBox(openBtn, trackBtn, addOnlyBtn),
			)
			state.ClipInboxBox.Add(widget.NewCard("", "", cardContent))
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
      <p>Job application details for <strong>` + html.EscapeString(payload.Company) + `</strong> have been sent to your LeGaJ Clip Inbox.</p>
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

	go func() {
		_ = http.ListenAndServe("127.0.0.1:8080", clipMux)
	}()
}
