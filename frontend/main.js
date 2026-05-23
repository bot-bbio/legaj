let selectedFile = null;

// Initialize on load
document.addEventListener("DOMContentLoaded", () => {
    setupDragAndDrop();
    loadProfileData();
    loadApplications();
    setupInitialPrepData();
});

// Tab Switching Logic
function switchTab(tabId) {
    document.querySelectorAll(".nav-item").forEach(btn => btn.classList.remove("active"));
    document.querySelectorAll(".tab-pane").forEach(pane => pane.classList.remove("active"));

    // Find clicked menu item and activate it
    const activeBtn = Array.from(document.querySelectorAll(".nav-item")).find(btn => 
        btn.getAttribute("onclick").includes(tabId)
    );
    if (activeBtn) activeBtn.classList.add("active");

    const targetPane = document.getElementById(`tab-${tabId}`);
    if (targetPane) targetPane.classList.add("active");
}

// Drag & Drop Setup
function setupDragAndDrop() {
    const dropZone = document.getElementById("drop-zone");
    const fileInput = document.getElementById("file-input");
    const fileNameDisplay = document.getElementById("file-name");

    dropZone.addEventListener("click", () => fileInput.click());

    fileInput.addEventListener("change", (e) => {
        if (e.target.files.length > 0) {
            selectedFile = e.target.files[0];
            fileNameDisplay.textContent = selectedFile.name;
        }
    });

    dropZone.addEventListener("dragover", (e) => {
        e.preventDefault();
        dropZone.classList.add("dragover");
    });

    dropZone.addEventListener("dragleave", () => {
        dropZone.classList.remove("dragover");
    });

    dropZone.addEventListener("drop", (e) => {
        e.preventDefault();
        dropZone.classList.remove("dragover");
        if (e.dataTransfer.files.length > 0) {
            selectedFile = e.dataTransfer.files[0];
            fileNameDisplay.textContent = selectedFile.name;
        }
    });
}

// REST API Calls

async function uploadResume() {
    if (!selectedFile) {
        alert("Please drop or select a resume file first.");
        return;
    }

    const btn = document.getElementById("btn-parse");
    btn.disabled = true;
    btn.textContent = "Parsing Resume...";

    const formData = new FormData();
    formData.append("resume", selectedFile);

    try {
        const response = await fetch("/api/parse-resume", {
            method: "POST",
            body: formData
        });

        if (!response.ok) {
            const errText = await response.text();
            throw new Error(errText);
        }

        const rawText = await response.text();
        
        // Populate standard mock structure to let user edit it
        const structuredProfile = {
            "personal_info": {
                "name": "Applicant Name",
                "email": "email@example.com",
                "phone": "000-000-0000",
                "location": "City, State",
                "linkedin": "linkedin.com/in/username",
                "website": "portfolio.com"
            },
            "target_roles": ["Product Manager"],
            "education": [
                {
                    "institution": "University Name",
                    "degree": "Degree",
                    "major": "Major",
                    "graduation_date": "May 2024",
                    "details": "Details..."
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
                        "Raw resume text ingested. Edit bullets here manually..."
                    ]
                }
            ],
            "skills": {
                "technical": ["Python", "Go", "SQL"]
            }
        };

        // Put parsed text under a default bullet point to let user structure it
        structuredProfile.experience[0].bullets = rawText.split('\n').filter(l => l.trim() !== "");
        
        document.getElementById("profile-data").value = JSON.stringify(structuredProfile, null, 2);
        alert("Resume successfully parsed! Please edit/review your JSON model details and click Save.");
    } catch (err) {
        alert("Error parsing resume: " + err.message);
    } finally {
        btn.disabled = false;
        btn.textContent = "Run Parse Engine";
    }
}

async function loadProfileData() {
    try {
        const response = await fetch("/references/user-profile.json");
        if (response.ok) {
            const data = await response.json();
            document.getElementById("profile-data").value = JSON.stringify(data, null, 2);
        }
    } catch (e) {
        // file doesn't exist yet, ignore
    }
}

async function saveProfileData() {
    const editor = document.getElementById("profile-data");
    try {
        // Verify JSON is valid
        const parsed = JSON.parse(editor.value);
        
        const response = await fetch("/api/save-profile", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(parsed)
        });

        if (response.ok) {
            alert("Profile successfully saved and updated!");
        } else {
            throw new Error(await response.text());
        }
    } catch (err) {
        alert("Failed to save. Ensure JSON syntax is valid: " + err.message);
    }
}

async function searchJobs() {
    const kw = document.getElementById("search-kw").value;
    const loc = document.getElementById("search-loc").value;

    if (!kw || !loc) {
        alert("Please fill out Keywords and Location filters.");
        return;
    }

    const linksBox = document.getElementById("search-links");
    linksBox.innerHTML = "<p class='placeholder-text'>Searching job directories...</p>";

    try {
        const response = await fetch("/api/search-jobs", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ keywords: kw, location: loc })
        });

        if (!response.ok) throw new Error(await response.text());

        const out = await response.text();
        
        // Parse links and draw cards
        linksBox.innerHTML = "";
        
        // Generate clickable link buttons
        const boards = [
            { name: "Google Jobs", url: `https://www.google.com/search?q=${encodeURIComponent(kw + ' jobs in ' + loc)}&ibp=htl;jobs` },
            { name: "LinkedIn", url: `https://www.linkedin.com/jobs/search/?keywords=${encodeURIComponent(kw)}&location=${encodeURIComponent(loc)}` },
            { name: "Indeed", url: `https://www.indeed.com/jobs?q=${encodeURIComponent(kw)}&l=${encodeURIComponent(loc)}` },
            { name: "ZipRecruiter", url: `https://www.ziprecruiter.com/jobs-search?search=${encodeURIComponent(kw)}&location=${encodeURIComponent(loc)}` }
        ];

        boards.forEach(b => {
            const card = document.createElement("a");
            card.className = "search-link-card";
            card.href = b.url;
            card.target = "_blank";
            card.innerHTML = `
                <h4>${b.name}</h4>
                <p>Click to open query in browser</p>
            `;
            linksBox.appendChild(card);
        });

        // Set doc center fields automatically to save manual entry
        document.getElementById("doc-role").value = kw;
    } catch (err) {
        linksBox.innerHTML = `<p class='placeholder-text' style='color:red;'>Search failed: ${err.message}</p>`;
    }
}

async function runTailoring() {
    const jd = document.getElementById("tailor-jd").value;
    if (!jd) {
        alert("Please paste the job description text.");
        return;
    }

    const diffBox = document.getElementById("tailor-diff");
    diffBox.textContent = "Analyzing job postings and comparing bullets...";

    try {
        // Load active user profile first
        const pResponse = await fetch("/references/user-profile.json");
        if (!pResponse.ok) throw new Error("Base user profile not found. Complete Setup first.");
        const profile = await pResponse.json();

        // Perform mock tailoring by simulating bullets updates
        // In real CLI, this calls the LLM, here we copy base to tailored and rewrite
        const tailored = JSON.parse(JSON.stringify(profile));
        if (tailored.experience && tailored.experience.length > 0) {
            tailored.experience[0].bullets[0] = `[Tailored] Successfully mapped experience details to align with the core requirements of this job posting.`;
        }

        const response = await fetch("/api/tailor-resume", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                job_description: jd,
                tailored_json: JSON.stringify(tailored)
            })
        });

        if (!response.ok) throw new Error(await response.text());

        const diffText = await response.text();
        diffBox.textContent = diffText;
        alert("Tailoring complete! user-profile-tailored.json drafted.");
    } catch (err) {
        diffBox.textContent = "Error: " + err.message;
        alert("Tailoring failed: " + err.message);
    }
}

async function compileResume() {
    const consoleBox = document.getElementById("doc-console");
    consoleBox.textContent = "Compiling PDF Resume...";

    try {
        const response = await fetch("/api/generate-resume", {
            method: "POST"
        });

        if (!response.ok) throw new Error(await response.text());

        consoleBox.textContent = await response.text();
        alert("Resume PDF successfully compiled!");
    } catch (err) {
        consoleBox.textContent = "Error: " + err.message;
    }
}

async function compileCoverLetter() {
    const comp = document.getElementById("doc-company").value;
    const role = document.getElementById("doc-role").value;
    const draft = document.getElementById("doc-letter").value;
    const consoleBox = document.getElementById("doc-console");

    if (!comp || !draft) {
        alert("Please enter Company Name and paste your Cover Letter text draft.");
        return;
    }

    consoleBox.textContent = "Compiling Cover Letter PDF...";

    try {
        const response = await fetch("/api/generate-cover-letter", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ company: comp, role: role, draft_text: draft })
        });

        if (!response.ok) throw new Error(await response.text());

        consoleBox.textContent = await response.text();
        alert("Cover Letter PDF compiled!");
    } catch (err) {
        consoleBox.textContent = "Error: " + err.message;
    }
}

// Tab 5 Prep

function setupInitialPrepData() {
    const defaultPrep = {
        "company": "Example Corp",
        "role": "Product Manager",
        "company_profile": "A leading SaaS data platform...",
        "elevator_pitch": "I am a PM specializing in analytics dashboard delivery...",
        "key_achievements": [
            "Launched analytics system boosting retention by 10%.",
            "Conducted user research to optimize onboarding checkout flow."
        ],
        "questions_to_ask": [
            "How do teams structure sprint cycles?",
            "What is the priority for the next quarter?"
        ],
        "questions_and_answers": [
            { "question": "Tell me about a project you led.", "answer": "At TechInnovate, I led the analytics portal dashboard launch..." }
        ],
        "flashcards": [
            { "question": "What is churn rate?", "answer": "Customers lost / total customers." }
        ]
    };
    document.getElementById("prep-json").value = JSON.stringify(defaultPrep, null, 2);
}

async function buildAnkiDeck() {
    const rawData = document.getElementById("prep-json").value;
    const consoleBox = document.getElementById("prep-console");
    consoleBox.textContent = "Generating Anki Deck Package (.apkg)...";

    try {
        // Validate JSON
        JSON.parse(rawData);

        const response = await fetch("/api/prep-interview", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ mode: "anki", data: rawData })
        });

        if (!response.ok) throw new Error(await response.text());

        consoleBox.textContent = await response.text();
        alert("Anki Deck package compiled successfully!");
    } catch (err) {
        consoleBox.textContent = "Error: " + err.message;
    }
}

async function buildCheatsheet() {
    const rawData = document.getElementById("prep-json").value;
    const consoleBox = document.getElementById("prep-console");
    consoleBox.textContent = "Compiling Cheatsheet Markdown...";

    try {
        // Validate JSON
        JSON.parse(rawData);

        const response = await fetch("/api/prep-interview", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ mode: "cheatsheet", data: rawData })
        });

        if (!response.ok) throw new Error(await response.text());

        consoleBox.textContent = await response.text();
        alert("Cheatsheet Markdown successfully written!");
    } catch (err) {
        consoleBox.textContent = "Error: " + err.message;
    }
}

// Tab 6 Tracker

async function loadApplications() {
    const tableBody = document.querySelector("#tracker-table tbody");
    tableBody.innerHTML = "<tr><td colspan='5' class='center-text'>Loading spreadsheet rows...</td></tr>";

    try {
        const response = await fetch("/api/applications");
        if (!response.ok) throw new Error(await response.text());

        const apps = await response.json();
        tableBody.innerHTML = "";

        if (apps.length === 0) {
            tableBody.innerHTML = "<tr><td colspan='5' class='center-text'>No job applications logged yet.</td></tr>";
            return;
        }

        apps.forEach(app => {
            const tr = document.createElement("tr");
            tr.innerHTML = `
                <td><b>${app.company}</b></td>
                <td>${app.role}</td>
                <td>${app.date}</td>
                <td><span class="status-badge ${app.status.toLowerCase()}">${app.status}</span></td>
                <td>${app.notes}</td>
            `;
            tableBody.appendChild(tr);
        });
    } catch (err) {
        tableBody.innerHTML = `<tr><td colspan='5' class='center-text' style='color:red;'>Failed to load tracker: ${err.message}</td></tr>`;
    }
}

async function logApplication() {
    const comp = document.getElementById("track-comp").value;
    const role = document.getElementById("track-role").value;
    const link = document.getElementById("track-link").value;

    if (!comp || !role) {
        alert("Please enter at least Company Name and Role.");
        return;
    }

    try {
        const response = await fetch("/api/applications", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                action: "add",
                company: comp,
                role: role,
                location: "Local",
                link: link,
                notes: "Logged via LeGaJ GUI"
            })
        });

        if (!response.ok) throw new Error(await response.text());

        alert("Application successfully logged to job-tracker.xlsx!");
        
        // Reset inputs
        document.getElementById("track-comp").value = "";
        document.getElementById("track-role").value = "";
        document.getElementById("track-link").value = "";
        
        loadApplications();
    } catch (err) {
        alert("Failed to log application: " + err.message);
    }
}

async function syncEmails() {
    const email = document.getElementById("imap-email").value;
    const pwd = document.getElementById("imap-pwd").value;
    const server = document.getElementById("imap-server").value;

    if (!email || !pwd || !server) {
        alert("Please configure all email sync details (Email, App Password, IMAP Server).");
        return;
    }

    try {
        const response = await fetch("/api/applications", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                action: "sync",
                email: email,
                password: pwd,
                imap_server: server
            })
        });

        if (!response.ok) throw new Error(await response.text());

        const logs = await response.text();
        alert("Email sync scan finished:\n" + logs);
        loadApplications();
    } catch (err) {
        alert("Failed to sync emails: " + err.message);
    }
}
