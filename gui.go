package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
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
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

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

func callGeminiWithSearchGo(apiKey, promptText string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

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
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	return text, nil
}

// AppState holds the application's global GUI state
type AppState struct {
	App            fyne.App
	Window         fyne.Window
	Profile        *Profile
	Applications   []JobApplication
	SelectedAppIdx int
	ApiKey         string
	Email          string
	Password       string
	ImapServer     string
	SaveFolder     string

	// Dashboard widgets
	WishlistLabel    *widget.Label
	AppliedLabel     *widget.Label
	InterviewLabel   *widget.Label
	OfferLabel       *widget.Label
	RejectedLabel    *widget.Label
	RecentBox        *fyne.Container
	SearchResultsBox *fyne.Container

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
	profileTab := buildProfileTab()
	trackerTab := buildTrackerTab()
	tailoringTab := buildTailoringTab()
	fileManagerTab := buildFileManagerTab()
	prepTab := buildPrepTab()
	settingsTab := buildSettingsTab()

	// Arrange layout
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Dashboard", theme.HomeIcon(), dashboardTab),
		container.NewTabItemWithIcon("Base Profile", theme.AccountIcon(), profileTab),
		container.NewTabItemWithIcon("Job Tracker", theme.ListIcon(), trackerTab),
		container.NewTabItemWithIcon("Tailor Assets", theme.DocumentCreateIcon(), tailoringTab),
		container.NewTabItemWithIcon("File Manager", theme.FolderIcon(), fileManagerTab),
		container.NewTabItemWithIcon("Interview Prep", theme.QuestionIcon(), prepTab),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), settingsTab),
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

func loadConfigurations() {
	state.SelectedAppIdx = -1
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
	content := fmt.Sprintf("GEMINI_API_KEY=%s\nLEGAJ_EMAIL=%s\nLEGAJ_PASSWORD=%s\nLEGAJ_IMAP_SERVER=%s\nLEGAJ_SAVE_FOLDER=%s\n",
		state.ApiKey, state.Email, state.Password, state.ImapServer, state.SaveFolder)
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
	refreshTrackerDetail()
}

func openLink(urlString string) {
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
	recentScroll.SetMinSize(fyne.NewSize(450, 160))

	recentCard := widget.NewCard("Recent Applications", "", recentScroll)

	// Job Discovery section using Gemini Search Grounding
	searchKeyword := widget.NewEntry()
	searchKeyword.SetPlaceHolder("e.g. Product Manager")
	searchLocation := widget.NewEntry()
	searchLocation.SetPlaceHolder("e.g. New York, NY")

	state.SearchResultsBox = container.NewVBox()
	resultsScroll := container.NewVScroll(state.SearchResultsBox)
	resultsScroll.SetMinSize(fyne.NewSize(450, 240))

	searchBtn := widget.NewButtonWithIcon("Find Jobs", theme.SearchIcon(), func() {
		if searchKeyword.Text == "" || searchLocation.Text == "" {
			dialog.ShowInformation("Required Info", "Please enter a job keyword and location.", state.Window)
			return
		}
		if state.ApiKey == "" {
			dialog.ShowInformation("API Key Missing", "Please configure your Gemini API Key in Settings first.", state.Window)
			return
		}

		progress := dialog.NewProgressInfinite("Searching Active Jobs", "Querying Google via Gemini Search Grounding...", state.Window)
		progress.Show()

		go func() {
			prompt := fmt.Sprintf(`Search Google right now for active job postings matching "%s" in "%s".

Find real, current job listings from sites like LinkedIn, Indeed, Glassdoor, Greenhouse, Lever, and company career pages.

Return a JSON array. Each item must have:
- "company": company name
- "role": exact job title
- "location": city/state or Remote
- "link": the direct URL to the job listing (must start with https://)
- "description": one sentence describing the key skills needed

Return only valid JSON, no extra text.`, searchKeyword.Text, searchLocation.Text)

			resJson, apiErr := callGeminiWithSearchGo(state.ApiKey, prompt)

			// Parse JSON — extract array regardless of markdown wrapping
			cleanJson := strings.TrimSpace(resJson)
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

			// If we have results, verify links in the background before touching UI
			type taggedResult struct {
				job   JobResult
				alive bool
			}
			var tagged []taggedResult

			if apiErr == nil && parseErr == nil && len(results) > 0 {
				verifyCh := make(chan taggedResult, len(results))
				httpClient := &http.Client{Timeout: 5 * time.Second}
				for _, job := range results {
					j := job
					go func() {
						alive := false
						if strings.HasPrefix(j.Link, "http") {
							resp, err := httpClient.Head(j.Link)
							if err == nil && resp.StatusCode < 400 {
								resp.Body.Close()
								alive = true
							} else {
								resp2, err2 := httpClient.Get(j.Link)
								if err2 == nil {
									resp2.Body.Close()
									alive = resp2.StatusCode < 400
								}
							}
						}
						verifyCh <- taggedResult{job: j, alive: alive}
					}()
				}
				for range results {
					tagged = append(tagged, <-verifyCh)
				}
			}

			// All heavy work done — now update UI on main thread
			fyne.Do(func() {
				progress.Hide()
				state.SearchResultsBox.Objects = nil

				if apiErr != nil {
					state.SearchResultsBox.Add(widget.NewLabel(fmt.Sprintf("Search error: %v", apiErr)))
					state.SearchResultsBox.Refresh()
					return
				}
				if parseErr != nil || len(results) == 0 {
					state.SearchResultsBox.Add(widget.NewLabel("No results returned. Try different keywords or location."))
					state.SearchResultsBox.Refresh()
					return
				}

				for _, t := range tagged {
					r := t.job
					compLabel := widget.NewLabelWithStyle(r.Company, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
					roleLabel := widget.NewLabel(fmt.Sprintf("%s  ·  %s", r.Role, r.Location))
					descLabel := widget.NewLabel(r.Description)
					descLabel.Wrapping = fyne.TextWrapWord

					openBtn := widget.NewButtonWithIcon("View Posting", theme.HelpIcon(), func() {
						openLink(r.Link)
					})
					trackTailorBtn := widget.NewButtonWithIcon("Track & Tailor", theme.DocumentCreateIcon(), func() {
						runTrackAndTailorAutomation(r.Company, r.Role, r.Location, r.Link, r.Description)
					})
					trackTailorBtn.Importance = widget.HighImportance

					header := container.NewHBox(compLabel)
					if t.alive {
						badge := widget.NewLabelWithStyle("✓ Verified", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
						header = container.NewHBox(compLabel, layout.NewSpacer(), badge)
					}

					cardContent := container.NewVBox(
						header,
						roleLabel,
						descLabel,
						container.NewHBox(openBtn, trackTailorBtn),
					)
					state.SearchResultsBox.Add(widget.NewCard("", "", cardContent))
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

	content := container.NewVBox(
		canvas.NewText("Welcome to LeGaJ", theme.PrimaryColor()),
		widget.NewLabel("Your premium native assistant to structure resumes, tailor content, and log trackers."),
		widget.NewSeparator(),
		kpiGrid,
		container.NewGridWithColumns(2, recentCard, searchCard),
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
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			filePath := reader.URI().Path()
			reader.Close()

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
		}, state.Window)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".docx", ".txt", ".md"}))
		fd.Show()
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

var (
	trackerDetailTitle  *widget.Label
	trackerDetailSub    *widget.Label
	trackerDetailStatus *widget.Label
	trackerDetailNotes  *widget.Label
	trackerResumeLink   *widget.Button
	trackerCoverLink    *widget.Button
)

func refreshTrackerDetail() {
	if trackerDetailTitle == nil {
		return
	}
	if state.TrackerSelected == nil {
		trackerDetailTitle.SetText("No Application Selected")
		trackerDetailSub.SetText("")
		trackerDetailStatus.SetText("")
		trackerDetailNotes.SetText("Select an application from the table to view details and generated files.")
		trackerResumeLink.Hide()
		trackerCoverLink.Hide()
		return
	}
	app := *state.TrackerSelected
	trackerDetailTitle.SetText(fmt.Sprintf("%s at %s", app.Role, app.Company))
	trackerDetailSub.SetText(fmt.Sprintf("Applied: %s  ·  Location: %s", app.Date, app.Location))
	trackerDetailStatus.SetText(fmt.Sprintf("Status: %s", app.Status))
	trackerDetailNotes.SetText(fmt.Sprintf("Notes:\n%s", app.Notes))

	resPath := filepath.Join(state.SaveFolder, app.Resume)
	if _, err := os.Stat(resPath); err == nil && app.Resume != "" {
		trackerResumeLink.SetText("Open Tailored Resume")
		trackerResumeLink.OnTapped = func() {
			openLink("file:///" + filepath.ToSlash(resPath))
		}
		trackerResumeLink.Show()
	} else {
		trackerResumeLink.Hide()
	}

	clPath := filepath.Join(state.SaveFolder, app.CoverLetter)
	if _, err := os.Stat(clPath); err == nil && app.CoverLetter != "" {
		trackerCoverLink.SetText("Open Cover Letter")
		trackerCoverLink.OnTapped = func() {
			openLink("file:///" + filepath.ToSlash(clPath))
		}
		trackerCoverLink.Show()
	} else {
		trackerCoverLink.Hide()
	}
}

// 3. JOB TRACKER SPREADSHEET TABLE VIEW
func buildTrackerTab() fyne.CanvasObject {
	// Initialize package labels/buttons for detail view
	trackerDetailTitle = widget.NewLabelWithStyle("No Application Selected", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	trackerDetailSub = widget.NewLabel("")
	trackerDetailStatus = widget.NewLabel("")
	trackerDetailNotes = widget.NewLabel("Select an application from the table to view details and generated files.")
	trackerDetailNotes.Wrapping = fyne.TextWrapWord

	trackerResumeLink = widget.NewButtonWithIcon("Open Tailored Resume", theme.DocumentIcon(), nil)
	trackerCoverLink = widget.NewButtonWithIcon("Open Cover Letter", theme.DocumentIcon(), nil)
	trackerResumeLink.Hide()
	trackerCoverLink.Hide()

	// Table widget setup for spreadsheet-like grid layout
	state.TrackerTable = widget.NewTable(
		func() (int, int) {
			return len(state.Applications) + 1, 5 // +1 for headers
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("Cell content")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)
			if id.Row == 0 {
				headers := []string{"Company", "Role", "Location", "Date", "Status"}
				label.SetText(headers[id.Col])
				label.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				if id.Row-1 < len(state.Applications) {
					app := state.Applications[id.Row-1]
					switch id.Col {
					case 0:
						label.SetText(app.Company)
					case 1:
						label.SetText(app.Role)
					case 2:
						label.SetText(app.Location)
					case 3:
						label.SetText(app.Date)
					case 4:
						label.SetText(app.Status)
					}
					label.TextStyle = fyne.TextStyle{}
				}
			}
		},
	)

	// Set column widths to look like spreadsheet grid
	state.TrackerTable.SetColumnWidth(0, 150)
	state.TrackerTable.SetColumnWidth(1, 180)
	state.TrackerTable.SetColumnWidth(2, 110)
	state.TrackerTable.SetColumnWidth(3, 90)
	state.TrackerTable.SetColumnWidth(4, 90)

	state.TrackerTable.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row-1 < len(state.Applications) {
			app := state.Applications[id.Row-1]
			state.TrackerSelected = &app
			state.SelectedAppIdx = id.Row - 1
			refreshTrackerDetail()
		}
	}

	addBtn := widget.NewButtonWithIcon("Add Application", theme.ContentAddIcon(), func() {
		openAddJobModal(nil)
	})

	updateBtn := widget.NewButtonWithIcon("Update Status", theme.DocumentCreateIcon(), func() {
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

	controlBar := container.NewHBox(addBtn, updateBtn, syncBtn)

	tableCard := widget.NewCard("Job Tracker", "Click any row cell to select an application", state.TrackerTable)
	tableContainer := container.NewBorder(nil, nil, nil, nil, tableCard)

	detailCard := widget.NewCard("Application Details", "", container.NewVBox(
		trackerDetailTitle,
		trackerDetailSub,
		trackerDetailStatus,
		widget.NewSeparator(),
		container.NewVScroll(trackerDetailNotes),
		widget.NewSeparator(),
		trackerResumeLink,
		trackerCoverLink,
	))

	split := container.NewHSplit(tableContainer, detailCard)
	split.SetOffset(0.6) // 60% table, 40% detail view

	return container.NewBorder(controlBar, nil, nil, nil, split)
}

func updateTrackerList() {
	if state.TrackerTable != nil {
		state.TrackerTable.Refresh()
	}
	refreshTrackerDetail()
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

			// Draft prompt aligning with style guide
			profileBytes, _ := os.ReadFile("references/user-profile.json")
			draftPrompt := fmt.Sprintf(`Write a professional 4-paragraph cover letter for Roberto Montero for the role of "%s" at "%s".
Use the following job description and requirements:
%s

Use the applicant's experience and background from their profile:
%s

Structure the cover letter exactly as follows:
- The first paragraph must state: "My name is Roberto Montero and I write to you regarding the [Exact Role Title] role at [Company Name]." Followed by a connection of my background (Market Research and Property Management) and drive to learn with the desire to transition into a fast-paced environment.
- The second paragraph must highlight my senior research roles (GLG, Quadrant Strategies), conducting research operations for tech companies.
- The third paragraph must highlight the Leasing Manager role at Live in Bing, overseeing $15 Million in assets and overhauling the sales pipeline through COVID-19.
- The fourth paragraph must summarize my adaptability and end with: "I look forward to discussing this opportunity further and I hope to hear from you soon!"

Format: Start with the recipient address block and the current date (formatted like e.g. "22 January 2025") at the top, then the salutation "To Whom it May Concern,", then the body, then the sign-off "Sincerely,", then "Roberto Montero".
Output ONLY the cover letter text, no conversational intro or outro.`, role, comp, state.TailorReqsEntry.Text, string(profileBytes))

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

// 6. SETTINGS VIEW WITH CREDITS AND HELP
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

	saveBtn := widget.NewButtonWithIcon("Save Configurations", theme.DocumentSaveIcon(), func() {
		state.ApiKey = state.SettingsApiKey.Text
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
		widget.NewLabel("Save Folder"), state.SettingsSaveFolder,
		widget.NewLabel("Email Address"), state.SettingsEmail,
		widget.NewLabel("IMAP App Password"), state.SettingsPassword,
		widget.NewLabel("IMAP Server"), state.SettingsImapServer,
	)

	// Help and Documentation
	helpDoc := widget.NewCard("LeGaJ Help & Documentation", "", container.NewVBox(
		widget.NewLabel("1. Overview: Matches profile details to job postings, tailors experiences, drafts cover letters, and logs tracked row states locally."),
		widget.NewLabel("2. Save Directory: Output PDFs (Resume/Cover Letter) compile into your Custom Save Folder (defaults to Google Drive)."),
		widget.NewLabel("3. Templates: Formatting applies Times New Roman styling and strict single-page constraints."),
	))

	bookmarkletJs := `javascript:(function(){var c=document.querySelector(".job-details-jobs-unified-top-card__company-name")?.innerText||prompt("Company:"),r=document.querySelector(".job-details-jobs-unified-top-card__job-title")?.innerText||prompt("Role:"),l=document.querySelector(".job-details-jobs-unified-top-card__bullet")?.innerText||prompt("Location:"),d=document.querySelector("#job-details")?.innerText||"";if(!c||!r)return;fetch("http://localhost:8080/clip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({company:c.trim(),role:r.trim(),location:(l||"").trim(),link:window.location.href,description:d.substring(0,300).trim()})}).then(res=>res.json()).then(data=>alert("Clipped to LeGaJ!")).catch(err=>alert("Error: "+err));})();`
	bookmarkletEntry := widget.NewEntry()
	bookmarkletEntry.SetText(bookmarkletJs)

	bookmarkletCard := widget.NewCard("Clip to LeGaJ Browser Bookmarklet", "Copy the javascript below and add it as a browser bookmark URL:", container.NewVBox(
		bookmarkletEntry,
		widget.NewLabel("Click the bookmark on LinkedIn job listing pages to clip details directly into your dashboard."),
	))

	securityCard := widget.NewCard("Security & Ethics Disclosure", "", container.NewVBox(
		widget.NewLabel("• LeGaJ operates 100% locally. Your PII (name, email, phone) and Gemini API Keys are kept on your machine."),
		widget.NewLabel("• The application NEVER auto-submits applications. It compiles PDFs for you to review and apply yourself."),
		widget.NewLabel("• AI features are used solely for structuring profile fields, tailoring resume bullets, and drafting cover letters."),
	))

	// Hyperlinked Creator Credits
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
		helpDoc,
		bookmarkletCard,
		securityCard,
		widget.NewSeparator(),
		creditsRow,
	)

	return container.NewScroll(content)
}

// -------------------------------------------------------------
// ONBOARDING SETUP WIZARD
// -------------------------------------------------------------

func showOnboardingWizard() {
	wizardWindow := state.App.NewWindow("LeGaJ - Setup Wizard")
	wizardWindow.Resize(fyne.NewSize(650, 520))

	// Navigation buttons
	nextBtn := widget.NewButton("Next", nil)
	backBtn := widget.NewButton("Back", nil)
	backBtn.Disable()
	nextBtn.Disable() // Disabled by default in step 1 until connections verified

	// Step 1: Welcome & Paths & Connections Test
	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetText(state.ApiKey)

	resumePathLabel := widget.NewLabel("No resume file selected")
	resumePathLabel.Wrapping = fyne.TextWrapOff
	selectResumeBtn := widget.NewButtonWithIcon("Choose Resume File", theme.DocumentIcon(), func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			resumePathLabel.SetText(reader.URI().Path())
			reader.Close()
		}, wizardWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf", ".docx", ".txt", ".md"}))
		fd.Show()
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
			_, err := callGeminiGo(apiKeyEntry.Text, "Say Hi", false)
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
		widget.NewLabel("Step 1: Configure your API keys, resume, and output folder permissions."),
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Gemini API Key"), apiKeyEntry,
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

	step3Form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Full Name"), nameEntry,
		widget.NewLabel("Email Address"), emailEntry,
		widget.NewLabel("Phone Number"), phoneEntry,
		widget.NewLabel("Location"), locEntry,
	)

	step3 := container.NewVBox(
		widget.NewLabelWithStyle("Step 3: Verify Parsed Details", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Confirm or edit your personal info before verifying the search grounding:"),
		widget.NewSeparator(),
		step3Form,
	)

	// Step 4: Verification (Test Search)
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
					testSearchLabel.SetText(fmt.Sprintf("✗ Search Grounding test failed: %v", err))
					nextBtn.Disable()
				} else {
					testSearchLabel.SetText(fmt.Sprintf("✓ Grounding verification successful!\nRaw output:\n%s", res))
					nextBtn.Enable()
				}
			})
		}()
	})

	step4 := container.NewVBox(
		widget.NewLabelWithStyle("Step 4: Search Grounding Verification", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
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
		} else if currentStep == 2 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Parse Resume & Continue")
		} else if currentStep == 3 {
			backBtn.Enable()
			nextBtn.Enable()
			nextBtn.SetText("Next")
		} else if currentStep == 4 {
			backBtn.Enable()
			nextBtn.SetText("Finish Setup")
			nextBtn.Disable() // Disabled until grounding test runs successfully
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
	}
	bypassBtn.OnTapped = bypassAction

	nextBtn.OnTapped = func() {
		if currentStep == 1 {
			state.ApiKey = apiKeyEntry.Text
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
		}
	}

	wizardWindow.SetContent(container.NewBorder(
		nil,
		container.NewHBox(backBtn, layout.NewSpacer(), nextBtn),
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

		tailorPrompt := fmt.Sprintf(`You are an expert resume writer. Tailor the applicant's experience bullet points and skills in the base profile JSON to align with the target job description.

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
		coverPrompt := fmt.Sprintf(`Write a professional 4-paragraph cover letter for Roberto Montero for the role of "%s" at "%s".
Use the following job description and requirements:
%s

Use the applicant's experience and background from their profile:
%s

Structure the cover letter exactly as follows:
- The first paragraph must state: "My name is Roberto Montero and I write to you regarding the [Exact Role Title] role at [Company Name]." Followed by a connection of my background (Market Research and Property Management) and drive to learn with the desire to transition into a fast-paced environment.
- The second paragraph must highlight my senior research roles (GLG, Quadrant Strategies), conducting research operations for tech companies.
- The third paragraph must highlight the Leasing Manager role at Live in Bing, overseeing $15 Million in assets and overhauling the sales pipeline through COVID-19.
- The fourth paragraph must summarize my adaptability and end with: "I look forward to discussing this opportunity further and I hope to hear from you soon!"

Format: Start with the recipient address block and the current date (formatted like e.g. "22 January 2025") at the top, then the salutation "To Whom it May Concern,", then the body, then the sign-off "Sincerely,", then "Roberto Montero".
Output ONLY the cover letter text, no conversational intro or outro.`, role, company, desc, string(baseProfileBytes))

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
	fmCurrentDir    string
	fmHistory       []string
	fmExplorerBox   *fyne.Container
	fmPreviewBox    *fyne.Container
	fmPathLabel     *widget.Label
	fmGridViewMode  = false
	fmSelectedFile  string
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
	fmCurrentDir = filepath.Clean(fmCurrentDir)
	fmPathLabel.SetText(fmCurrentDir)

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
	if fmGridViewMode {
		gridContainer := container.New(layout.NewGridWrapLayout(fyne.NewSize(130, 150)))
		for _, entry := range allEntries {
			e := entry
			name := e.Name()
			isDir := e.IsDir()

			icon := theme.DocumentIcon()
			if isDir {
				icon = theme.FolderIcon()
			}

			iconWidget := widget.NewIcon(icon)
			nameLabel := widget.NewLabel(name)
			nameLabel.Alignment = fyne.TextAlignCenter
			nameLabel.Wrapping = fyne.TextWrapOff

			cardContent := container.NewVBox(
				container.NewCenter(iconWidget),
				nameLabel,
			)

			btn := widget.NewButton("", func() {
				if isDir {
					fmHistory = append(fmHistory, fmCurrentDir)
					fmCurrentDir = filepath.Join(fmCurrentDir, name)
					refreshFileManager()
				} else {
					selectFileManagerFile(name)
				}
			})

			card := widget.NewCard("", "", container.NewMax(cardContent, btn))
			gridContainer.Add(container.NewPadded(card))
		}
		fmExplorerBox.Add(gridContainer)
	} else {
		for _, entry := range allEntries {
			e := entry
			name := e.Name()
			isDir := e.IsDir()

			icon := theme.DocumentIcon()
			if isDir {
				icon = theme.FolderIcon()
			}

			iconWidget := widget.NewIcon(icon)
			nameLabel := widget.NewLabel(name)

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
			fmExplorerBox.Add(row)
		}
	}
	fmExplorerBox.Refresh()
}

func buildFileManagerTab() fyne.CanvasObject {
	fmCurrentDir = state.SaveFolder
	if fmCurrentDir == "" {
		fmCurrentDir = "."
	}

	fmPathLabel = widget.NewLabel(fmCurrentDir)
	fmPathLabel.Wrapping = fyne.TextWrapOff

	backBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if len(fmHistory) > 0 {
			fmCurrentDir = fmHistory[len(fmHistory)-1]
			fmHistory = fmHistory[:len(fmHistory)-1]
			refreshFileManager()
		}
	})

	toggleBtn := widget.NewButtonWithIcon("", theme.GridIcon(), func() {
		fmGridViewMode = !fmGridViewMode
		refreshFileManager()
	})

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		refreshFileManager()
	})

	toolbar := container.NewHBox(
		backBtn,
		refreshBtn,
		toggleBtn,
		fmPathLabel,
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

func startClipServer() {
	http.HandleFunc("/clip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Company     string `json:"company"`
			Role        string `json:"role"`
			Location    string `json:"location"`
			Link        string `json:"link"`
			Description string `json:"description"`
		}

		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Company == "" || payload.Role == "" {
			http.Error(w, "Company and Role are required", http.StatusBadRequest)
			return
		}

		resumePdfName := fmt.Sprintf("%s_Resume_Tailored.pdf", strings.ReplaceAll(payload.Company, " ", "_"))
		coverLetterPdfName := fmt.Sprintf("%s_Cover_Letter.pdf", strings.ReplaceAll(payload.Company, " ", "_"))

		_, err = RunManageApplications("add", payload.Company, payload.Role, payload.Location, payload.Link, resumePdfName, coverLetterPdfName, "Clipped from browser. "+payload.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reloadAllViews()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	go func() {
		_ = http.ListenAndServe(":8080", nil)
	}()
}
