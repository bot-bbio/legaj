package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var forceWizard bool

func main() {
	// Initialize directories
	os.MkdirAll("outputs", 0755)
	os.MkdirAll("references", 0755)

	if len(os.Args) < 2 {
		startFyneGUI()
		return
	}

	command := os.Args[1]

	switch command {
	case "wizard":
		forceWizard = true
		startFyneGUI()
		return

	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)

	case "parse-resume":
		handleParseResumeCmd()

	case "tailor-resume":
		handleTailorResumeCmd()

	case "design-resume":
		handleDesignResumeCmd()

	case "write-cover-letter":
		handleWriteCoverLetterCmd()

	case "prep-interview":
		handlePrepInterviewCmd()

	case "search-jobs":
		handleSearchJobsCmd()

	case "manage-apps":
		handleManageAppsCmd()

	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("LeGaJ - Let's Get a Job CLI Tool")
	fmt.Println("\nUsage:")
	fmt.Println("  legaj <command> [arguments]")
	fmt.Println("\nAvailable Commands:")
	fmt.Println("  wizard                Launch the step-by-step onboarding setup wizard.")
	fmt.Println("  parse-resume          Extract text from a resume file (PDF/DOCX/TXT/MD).")
	fmt.Println("  tailor-resume         Compare a base profile with a tailored profile.")
	fmt.Println("  design-resume         Compile a profile JSON into a PDF resume.")
	fmt.Println("  write-cover-letter    Compile a cover letter PDF from a profile and a draft.")
	fmt.Println("  prep-interview        Generate study flashcards or cheatsheets.")
	fmt.Println("  search-jobs           Find active job listings and generate search URLs.")
	fmt.Println("  manage-apps           Manage the job applications tracker (Excel).")
	fmt.Println("\nUse \"legaj <command> -h\" for more information about a command.")
}

func handleParseResumeCmd() {
	cmd := flag.NewFlagSet("parse-resume", flag.ExitOnError)
	resumePath := cmd.String("resume", "", "Path to the resume file (PDF/DOCX/TXT/MD) [Required]")
	outputPath := cmd.String("output", "", "Optional path to save base JSON profile")

	cmd.Parse(os.Args[2:])

	if *resumePath == "" {
		fmt.Println("Error: -resume parameter is required")
		cmd.Usage()
		os.Exit(1)
	}

	outText, err := RunParseResume(*resumePath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)

	if *outputPath != "" {
		// Create base profile structure skeleton
		lines := strings.Split(outText, "\n")
		var bullets []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				bullets = append(bullets, trimmed)
			}
		}

		profile := map[string]interface{}{
			"personal_info": map[string]string{
				"name":     "Roberto Montero",
				"email":    "0.Roberto.Montero@gmail.com",
				"phone":    "813.597.5308",
				"location": "Bronx, NY",
				"linkedin": "linkedin.com/in/roberto-montero",
				"website":  "robertomontero.dev",
			},
			"target_roles": []string{"Product Manager"},
			"education": []map[string]string{
				{
					"institution":     "University",
					"degree":          "Degree",
					"major":           "Major",
					"graduation_date": "Date",
					"location":        "Location",
					"gpa":             "GPA",
					"details":         "Details",
				},
			},
			"experience": []map[string]interface{}{
				{
					"company":    "Company",
					"role":       "Role",
					"location":   "Location",
					"start_date": "Start",
					"end_date":   "End",
					"bullets":    bullets,
				},
			},
			"projects": []map[string]interface{}{
				{
					"name":         "Project",
					"description":  "Description",
					"technologies": []string{"Python"},
					"details":      "Details",
				},
			},
			"skills": map[string][]string{
				"technical": {"Python", "SQL"},
			},
		}

		jsonData, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			fmt.Printf("Error creating JSON: %v\n", err)
			os.Exit(1)
		}

		err = os.WriteFile(*outputPath, jsonData, 0644)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nSaved base profile JSON skeleton to: %s\n", *outputPath)
	}
}

func handleTailorResumeCmd() {
	cmd := flag.NewFlagSet("tailor-resume", flag.ExitOnError)
	basePath := cmd.String("base", "references/user-profile.json", "Path to base JSON profile")
	tailoredPath := cmd.String("tailored", "references/user-profile-tailored.json", "Path to tailored JSON profile")

	cmd.Parse(os.Args[2:])

	outText, err := RunTailorResume(*basePath, *tailoredPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)
}

func handleDesignResumeCmd() {
	cmd := flag.NewFlagSet("design-resume", flag.ExitOnError)
	profilePath := cmd.String("profile", "", "Path to profile JSON. Defaults to user-profile-tailored.json if it exists, otherwise user-profile.json")
	outputPath := cmd.String("output", "outputs/Resume_Generated.pdf", "Output path for the generated PDF")

	cmd.Parse(os.Args[2:])

	selectedProfile := *profilePath
	if selectedProfile == "" {
		if _, err := os.Stat("references/user-profile-tailored.json"); err == nil {
			selectedProfile = "references/user-profile-tailored.json"
		} else {
			selectedProfile = "references/user-profile.json"
		}
	}

	outText, err := RunGenerateResume(selectedProfile, *outputPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)
	fmt.Printf("Saved PDF resume to: %s\n", *outputPath)
}

func handleWriteCoverLetterCmd() {
	cmd := flag.NewFlagSet("write-cover-letter", flag.ExitOnError)
	company := cmd.String("company", "", "Target Company Name [Required]")
	role := cmd.String("role", "", "Target Role / Job Title")
	draft := cmd.String("draft", "", "Draft cover letter text or path to a draft text file [Required]")
	profilePath := cmd.String("profile", "references/user-profile.json", "Path to base JSON profile")
	outputPath := cmd.String("output", "", "Output path for the generated Cover Letter PDF")

	cmd.Parse(os.Args[2:])

	if *company == "" || *draft == "" {
		fmt.Println("Error: -company and -draft parameters are required")
		cmd.Usage()
		os.Exit(1)
	}

	if *role != "" {
		fmt.Printf("Role specified: %s\n", *role)
	}

	// Determine if draft is a file path or raw text
	var draftPath string
	if _, err := os.Stat(*draft); err == nil {
		// It's an existing file path
		draftPath = *draft
	} else {
		// It's raw text; save to a temporary file
		tempPath := filepath.Join("outputs", "temp_cli_draft.txt")
		err := os.WriteFile(tempPath, []byte(*draft), 0644)
		if err != nil {
			fmt.Printf("Error writing temporary draft file: %v\n", err)
			os.Exit(1)
		}
		draftPath = tempPath
		defer os.Remove(tempPath)
	}

	selectedOutput := *outputPath
	if selectedOutput == "" {
		compClean := strings.ReplaceAll(*company, " ", "_")
		selectedOutput = fmt.Sprintf("outputs/%s_Cover_Letter.pdf", compClean)
	}

	outText, err := RunGenerateCoverLetter(*profilePath, draftPath, selectedOutput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)
	fmt.Printf("Saved Cover Letter PDF to: %s\n", selectedOutput)
}

func handlePrepInterviewCmd() {
	cmd := flag.NewFlagSet("prep-interview", flag.ExitOnError)
	dataPath := cmd.String("data", "", "Path to the prep JSON data file [Required]")
	mode := cmd.String("mode", "all", "Prep mode: anki, cheatsheet, or all")

	cmd.Parse(os.Args[2:])

	if *dataPath == "" {
		fmt.Println("Error: -data parameter is required")
		cmd.Usage()
		os.Exit(1)
	}

	outText, err := RunPrepareInterview(*dataPath, *mode)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)
}

func handleSearchJobsCmd() {
	cmd := flag.NewFlagSet("search-jobs", flag.ExitOnError)
	keywords := cmd.String("keywords", "", "Role keywords to search for [Required]")
	location := cmd.String("location", "", "Job location [Required]")
	outputPath := cmd.String("output", "outputs/job_search_results.json", "Path to save search results JSON")

	cmd.Parse(os.Args[2:])

	if *keywords == "" || *location == "" {
		fmt.Println("Error: -keywords and -location parameters are required")
		cmd.Usage()
		os.Exit(1)
	}

	outText, err := RunSearchJobs(*keywords, *location)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(outText)
	fmt.Printf("Job search results saved to: %s\n", *outputPath)
}

func handleManageAppsCmd() {
	if len(os.Args) < 3 {
		printManageAppsUsage()
		os.Exit(1)
	}

	action := os.Args[2]

	switch action {
	case "list":
		outText, err := RunManageApplications("list")
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		// Print applications in structured list/JSON or format
		var apps []map[string]interface{}
		err = json.Unmarshal([]byte(outText), &apps)
		if err != nil {
			// Fallback to raw output
			fmt.Println(outText)
		} else {
			fmt.Println("Current Tracked Applications:")
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("%-20s %-25s %-12s %-12s %-15s\n", "Company", "Role", "Applied Date", "Status", "Location")
			fmt.Println("--------------------------------------------------------------------------------")
			for _, app := range apps {
				fmt.Printf("%-20s %-25s %-12s %-12s %-15s\n", 
					limitString(fmt.Sprintf("%v", app["company"]), 20), 
					limitString(fmt.Sprintf("%v", app["role"]), 25), 
					app["date"], 
					app["status"], 
					limitString(fmt.Sprintf("%v", app["location"]), 15))
			}
			fmt.Println("--------------------------------------------------------------------------------")
		}

	case "add":
		cmd := flag.NewFlagSet("manage-apps add", flag.ExitOnError)
		company := cmd.String("company", "", "Company Name [Required]")
		role := cmd.String("role", "", "Job Role [Required]")
		location := cmd.String("location", "Local", "Job Location")
		link := cmd.String("link", "", "Job Link URL")
		resume := cmd.String("resume", "", "Resume Used")
		coverLetter := cmd.String("cover-letter", "", "Cover Letter Used")
		notes := cmd.String("notes", "", "Notes")

		cmd.Parse(os.Args[3:])

		if *company == "" || *role == "" {
			fmt.Println("Error: -company and -role parameters are required for 'add'")
			cmd.Usage()
			os.Exit(1)
		}

		outText, err := RunManageApplications("add", *company, *role, *location, *link, *resume, *coverLetter, *notes)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(outText)

	case "update":
		cmd := flag.NewFlagSet("manage-apps update", flag.ExitOnError)
		company := cmd.String("company", "", "Company Name [Required]")
		role := cmd.String("role", "", "Job Role [Required]")
		status := cmd.String("status", "", "New Status [Required]")
		notes := cmd.String("notes", "", "Notes to append")

		cmd.Parse(os.Args[3:])

		if *company == "" || *role == "" || *status == "" {
			fmt.Println("Error: -company, -role, and -status parameters are required for 'update'")
			cmd.Usage()
			os.Exit(1)
		}

		outText, err := RunManageApplications("update", *company, *role, *status, *notes)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(outText)

	case "sync":
		cmd := flag.NewFlagSet("manage-apps sync", flag.ExitOnError)
		email := cmd.String("email", "", "Email address [Required]")
		password := cmd.String("password", "", "App password [Required]")
		imap := cmd.String("imap", "", "IMAP Server host [Required]")

		cmd.Parse(os.Args[3:])

		if *email == "" || *password == "" || *imap == "" {
			fmt.Println("Error: -email, -password, and -imap parameters are required for 'sync'")
			cmd.Usage()
			os.Exit(1)
		}

		outText, err := RunManageApplications("sync", *email, *password, *imap)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(outText)

	default:
		fmt.Printf("Unknown manage-apps action: %s\n\n", action)
		printManageAppsUsage()
		os.Exit(1)
	}
}

func printManageAppsUsage() {
	fmt.Println("Usage:")
	fmt.Println("  legaj manage-apps <action> [arguments]")
	fmt.Println("\nActions:")
	fmt.Println("  list                  List all currently tracked job applications.")
	fmt.Println("  add [flags]           Add a new job application to references/job-tracker.xlsx.")
	fmt.Println("  update [flags]        Update the status of an existing job application.")
	fmt.Println("  sync [flags]          Automatically sync status updates from your email inbox.")
}

func limitString(s string, limit int) string {
	if len(s) > limit {
		return s[:limit-3] + "..."
	}
	return s
}

