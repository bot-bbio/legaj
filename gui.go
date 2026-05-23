package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
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

// AppState holds the application's global GUI state
type AppState struct {
	App             fyne.App
	Window          fyne.Window
	Profile         *Profile
	Applications    []JobApplication
	SelectedAppIdx  int
	ApiKey          string
	Email           string
	Password        string
	ImapServer      string
	
	// Dashboard widgets
	WishlistLabel   *widget.Label
	AppliedLabel    *widget.Label
	InterviewLabel  *widget.Label
	OfferLabel      *widget.Label
	RejectedLabel   *widget.Label
	RecentList      *container.AppTabs // Tab or box
	RecentBox       *fyne.Container

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
	TrackerList     *widget.List
	TrackerSelected *JobApplication

	// Tailoring widgets
	TailorJobSelect *widget.Select
	TailorReqsEntry *widget.Entry
	OriginalPreview *widget.Label
	TailoredPreview *widget.Label
	TailorCompare   *fyne.Container

	// Prep widgets
	PrepJobSelect   *widget.Select
	PrepStatus      *widget.Label
	FlashcardBox    *fyne.Container
	CardQuestion    *widget.Label
	CardAnswer      *widget.Label
	CardIndicator   *widget.Label
	Flashcards      []Flashcard
	CurrentCardIdx  int

	// Settings widgets
	SettingsApiKey     *widget.Entry
	SettingsEmail      *widget.Entry
	SettingsPassword   *widget.Entry
	SettingsImapServer *widget.Entry
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

	// Load stored configurations (.env and JSON structures)
	loadConfigurations()
	loadProfileData()
	loadTrackerData()

	// Initialize UI sections
	dashboardTab := buildDashboardTab()
	profileTab := buildProfileTab()
	trackerTab := buildTrackerTab()
	tailoringTab := buildTailoringTab()
	prepTab := buildPrepTab()
	settingsTab := buildSettingsTab()

	// Arrange layout
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Dashboard", theme.HomeIcon(), dashboardTab),
		container.NewTabItemWithIcon("Base Profile", theme.AccountIcon(), profileTab),
		container.NewTabItemWithIcon("Job Tracker", theme.ListIcon(), trackerTab),
		container.NewTabItemWithIcon("Tailor Assets", theme.DocumentCreateIcon(), tailoringTab),
		container.NewTabItemWithIcon("Interview Prep", theme.QuestionIcon(), prepTab),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), settingsTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	state.Window.SetContent(tabs)
	state.Window.ShowAndRun()
}

// -------------------------------------------------------------
// Data Load/Save Helpers
// -------------------------------------------------------------

func loadConfigurations() {
	// Simple .env parser
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
				}
			}
		}
	}
}

func saveConfigurations() error {
	content := fmt.Sprintf("GEMINI_API_KEY=%s\nLEGAJ_EMAIL=%s\nLEGAJ_PASSWORD=%s\nLEGAJ_IMAP_SERVER=%s\n",
		state.ApiKey, state.Email, state.Password, state.ImapServer)
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
	loadProfileData()
	loadTrackerData()
	updateDashboardStats()
	updateTrackerList()
	updateDropdownSelectors()
}

// -------------------------------------------------------------
// View Builders
// -------------------------------------------------------------

// 1. DASHBOARD VIEW
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
		createKPICard("Offers Recieved", state.OfferLabel, color.RGBA{R: 16, G: 185, B: 129, A: 255}),
		createKPICard("Rejected", state.RejectedLabel, color.RGBA{R: 239, G: 68, B: 68, A: 255}),
	)

	state.RecentBox = container.NewVBox()
	recentScroll := container.NewVScroll(state.RecentBox)
	recentScroll.SetMinSize(fyne.NewSize(450, 300))

	recentCard := widget.NewCard("Recent Job Applications", "", recentScroll)

	tipsCard := widget.NewCard("Job Search Best Practices", "", container.NewVBox(
		widget.NewLabel("🎯 Tailor for each application: Use the Document Tailoring tab to rewrite your bullets."),
		widget.NewLabel("⏰ Log submissions: Update statuses immediately so your numbers match real progress."),
		widget.NewLabel("🧠 Practice Mock Interviews: Review flashcards prior to scheduling hiring calls."),
	))

	updateDashboardStats()

	content := container.NewVBox(
		canvas.NewText("Welcome to LeGaJ", theme.PrimaryColor()),
		widget.NewLabel("Your premium native assistant to structure resumes, tailor content, and log trackers."),
		widget.NewSeparator(),
		kpiGrid,
		container.NewGridWithColumns(2, recentCard, tipsCard),
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
		case "Rejected":
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
      "degree": "Degree (e.g. BS, BA)",
      "major": "Major subject",
      "graduation_date": "Month Year or Year",
      "location": "City, State",
      "gpa": "GPA (optional)",
      "details": "Awards, honors, or description (optional)"
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
  "projects": [
    {
      "name": "Project Name",
      "description": "Short description of project",
      "technologies": ["Tech 1", "Tech 2"],
      "details": "Bullet point of achievements or specifics"
    }
  ],
  "skills": {
    "technical": ["Python", "SQL", "etc."],
    "product_management": ["Agile", "Figma", "etc."]
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
		widget.NewLabel("LinkedIn URL"), state.LinkedinEntry,
		widget.NewLabel("Website URL"), state.WebsiteEntry,
		widget.NewLabel("Target Roles (csv)"), state.RolesEntry,
		widget.NewLabel("Technical Skills (csv)"), state.TechSkillsEntry,
		widget.NewLabel("Other Skills (csv)"), state.PmSkillsEntry,
	)

	// Experience lists with add buttons
	addExpBtn := widget.NewButton("+ Add Experience", func() {
		state.Profile.Experience = append(state.Profile.Experience, Experience{Company: "New Company", Role: "Role", Location: "", StartDate: "", EndDate: "", Bullets: []string{}})
		renderExperienceForm()
	})
	addEduBtn := widget.NewButton("+ Add Education", func() {
		state.Profile.Education = append(state.Profile.Education, Education{Institution: "University", Degree: "", Major: "", GraduationDate: "", GPA: "", Details: ""})
		renderEducationForm()
	})
	addProjBtn := widget.NewButton("+ Add Project", func() {
		state.Profile.Projects = append(state.Profile.Projects, Project{Name: "New Project", Description: "", Technologies: []string{}, Details: ""})
		renderProjectForm()
	})

	mainScroll := container.NewVScroll(container.NewVBox(
		container.NewHBox(canvas.NewText("Base Profile Information", theme.PrimaryColor()), layout.NewSpacer(), importBtn),
		contactForm,
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Professional Experience", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), addExpBtn),
		state.ExpContainer,
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Education History", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), addEduBtn),
		state.EduContainer,
		widget.NewSeparator(),
		container.NewHBox(widget.NewLabelWithStyle("Projects", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), addProjBtn),
		state.ProjContainer,
		widget.NewSeparator(),
		saveBtn,
	))
	mainScroll.SetMinSize(fyne.NewSize(0, 500))
	return mainScroll
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
	
	if tech, ok := state.Profile.Skills["technical"]; ok {
		state.TechSkillsEntry.SetText(strings.Join(tech, ", "))
	}
	if other, ok := state.Profile.Skills["product_management"]; ok {
		state.PmSkillsEntry.SetText(strings.Join(other, ", "))
	}

	renderExperienceForm()
	renderEducationForm()
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
		datesEnt.SetText(fmt.Sprintf("%s - %s", exp.StartDate, exp.EndDate))
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
			widget.NewLabel("Bullets (Line-by-line)"), bulletsEnt,
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

// 3. JOB TRACKER VIEW
func buildTrackerTab() fyne.CanvasObject {
	state.TrackerList = widget.NewList(
		func() int {
			return len(state.Applications)
		},
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.DocumentIcon())
			compLabel := widget.NewLabelWithStyle("Company", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			roleLabel := widget.NewLabel("Role / Location")
			statusLabel := widget.NewLabel("Applied")
			return container.NewHBox(icon, container.NewVBox(compLabel, roleLabel), layout.NewSpacer(), statusLabel)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			app := state.Applications[id]
			box := item.(*fyne.Container)
			vbox := box.Objects[1].(*fyne.Container)
			
			vbox.Objects[0].(*widget.Label).SetText(app.Company)
			vbox.Objects[1].(*widget.Label).SetText(fmt.Sprintf("%s (%s)", app.Role, app.Location))
			box.Objects[3].(*widget.Label).SetText(app.Status)
		},
	)

	state.TrackerList.OnSelected = func(id widget.ListItemID) {
		app := state.Applications[id]
		state.TrackerSelected = &app
		state.SelectedAppIdx = id
	}

	addBtn := widget.NewButtonWithIcon("Add Application", theme.ContentAddIcon(), func() {
		openAddJobModal(nil)
	})

	updateBtn := widget.NewButtonWithIcon("Update Status", theme.DocumentCreateIcon(), func() {
		if state.TrackerSelected == nil {
			dialog.ShowInformation("Selection Required", "Please select a job application card first.", state.Window)
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
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}
			dialog.ShowInformation("Sync complete", outText, state.Window)
			reloadAllViews()
		}()
	})

	controlBar := container.NewHBox(addBtn, updateBtn, syncBtn)
	
	listCard := widget.NewCard("Tracked Job Cards", "Select any job card to perform details updates", state.TrackerList)
	listCardContainer := container.NewBorder(nil, nil, nil, nil, listCard)

	return container.NewBorder(controlBar, nil, nil, nil, listCardContainer)
}

func updateTrackerList() {
	if state.TrackerList != nil {
		state.TrackerList.Refresh()
	}
}

func openAddJobModal(job *JobApplication) {
	compEnt := widget.NewEntry()
	roleEnt := widget.NewEntry()
	locEnt := widget.NewEntry()
	dateEnt := widget.NewEntry()
	linkEnt := widget.NewEntry()
	notesEnt := widget.NewEntry()
	
	statusSelect := widget.NewSelect([]string{"Wishlist", "Applied", "Interviewing", "Offer", "Rejected"}, nil)
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
				// Update
				_, err = RunManageApplications("update", compEnt.Text, roleEnt.Text, statusSelect.Selected, notesEnt.Text)
			} else {
				// Add
				_, err = RunManageApplications("add", compEnt.Text, roleEnt.Text, locEnt.Text, linkEnt.Text, "Resume_Generated.pdf", compEnt.Text+"_Cover_Letter.pdf", notesEnt.Text)
			}
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}
			reloadAllViews()
		}()
	}, state.Window)
	d.Resize(fyne.NewSize(500, 450))
	d.Show()
}

// 4. DOCUMENT TAILORING VIEW
func buildTailoringTab() fyne.CanvasObject {
	state.TailorJobSelect = widget.NewSelect([]string{}, func(selected string) {
		// prefill requirement field or original text if found
	})
	updateDropdownSelectors()

	state.TailorReqsEntry = widget.NewMultiLineEntry()
	state.TailorReqsEntry.SetPlaceHolder("Paste job requirements / description responsibilities here...")
	state.TailorReqsEntry.SetMinRowsVisible(6)

	state.OriginalPreview = widget.NewLabel("Original bullet points will appear here.")
	state.TailoredPreview = widget.NewLabel("Tailored bullet points will appear here.")
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

		go func() {
			// Trigger Python tailor script
			outText, err := RunTailorResume("references/user-profile.json", "references/user-profile-tailored.json")
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}

			// Parse original vs tailored comparison to show in preview labels
			state.OriginalPreview.SetText("See references/user-profile.json for original experiences.")
			state.TailoredPreview.SetText(outText)
			state.TailorCompare.Show()
			state.TailorCompare.Refresh()
		}()
	})

	compileResumeBtn := widget.NewButtonWithIcon("Compile Tailored PDF", theme.DocumentIcon(), func() {
		progress := dialog.NewProgressInfinite("Compiling PDF", "Generating publication-quality PDF via ReportLab...", state.Window)
		progress.Show()

		go func() {
			outputPath := "outputs/Resume_Generated_Tailored.pdf"
			outText, err := RunGenerateResume("references/user-profile-tailored.json", outputPath)
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}
			dialog.ShowInformation("PDF Generated", fmt.Sprintf("%s\nSaved to: %s", outText, outputPath), state.Window)
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
			outputPath := fmt.Sprintf("outputs/%s_Cover_Letter.pdf", strings.ReplaceAll(comp, " ", "_"))
			// Draft standard draft prompt to CLI
			draftPrompt := fmt.Sprintf("Drafting cover letter for %s role at %s based on requirements: %s", role, comp, state.TailorReqsEntry.Text)
			
			outText, err := RunGenerateCoverLetter("references/user-profile.json", draftPrompt, outputPath)
			progress.Hide()
			if err != nil {
				dialog.ShowError(err, state.Window)
				return
			}
			dialog.ShowInformation("Cover Letter Generated", fmt.Sprintf("%s\nSaved to: %s", outText, outputPath), state.Window)
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
			// Save simple mock JSON data to trigger prep script
			parts := strings.SplitN(state.PrepJobSelect.Selected, " - ", 2)
			comp := parts[0]
			role := ""
			if len(parts) == 2 {
				role = parts[1]
			}

			// Draft basic template prep json
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

	state.SettingsApiKey.SetText(state.ApiKey)
	state.SettingsEmail.SetText(state.Email)
	state.SettingsPassword.SetText(state.Password)
	state.SettingsImapServer.SetText(state.ImapServer)

	saveBtn := widget.NewButtonWithIcon("Save Configurations", theme.DocumentSaveIcon(), func() {
		state.ApiKey = state.SettingsApiKey.Text
		state.Email = state.SettingsEmail.Text
		state.Password = state.SettingsPassword.Text
		state.ImapServer = state.SettingsImapServer.Text

		err := saveConfigurations()
		if err != nil {
			dialog.ShowError(err, state.Window)
			return
		}
		dialog.ShowInformation("Success", "Settings written to secure local .env configuration.", state.Window)
	})

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Gemini API Key"), state.SettingsApiKey,
		widget.NewLabel("Email Address"), state.SettingsEmail,
		widget.NewLabel("IMAP App Password"), state.SettingsPassword,
		widget.NewLabel("IMAP Server"), state.SettingsImapServer,
	)

	content := container.NewVBox(
		canvas.NewText("Settings & Configurations", theme.PrimaryColor()),
		widget.NewLabel("Manage local API keys and configurations securely stored in your .env file."),
		form,
		saveBtn,
	)

	return container.NewScroll(content)
}
