# LeGaJ Clip Tool Test Suite

This document defines the test suite designed to expose and verify potential bugs and edge cases in the LeGaJ Clipper tool.

---

## Test Cases

### TC-01: Port Conflict Handling
*   **Goal:** Verify how the application behaves when port `8080` is already in use by another service.
*   **Preconditions:** A local server (e.g., Python HTTP server) is already running on port `8080`.
*   **Execution Steps:**
    1. Open PowerShell and start a mock listener:
       ```powershell
       $listener = [System.Net.Sockets.TcpListener]8080
       $listener.Start()
       ```
    2. Start the LeGaJ application (`go run .` or launch `legaj.exe`).
    3. Observe the stdout/stderr console and the GUI startup behavior.
*   **Expected Behavior:** The application should catch the socket error, log a warning/error (e.g., `"Port 8080 already in use, clip server disabled"`), and continue loading the GUI without panicking or crashing.

---

### TC-02: Mixed Content and Popup Blocker Fallback
*   **Goal:** Verify that browsers correctly execute requests from secure (`https://`) origins and handle popup blocking.
*   **Preconditions:** LeGaJ is running on port `8080`. Open a secure website (e.g., `https://example.com`) in Chrome.
*   **Execution Steps:**
    1. Open the Developer Tools console (F12) on the secure site.
    2. Run a fetch query to the clip server:
       ```javascript
       fetch('http://127.0.0.1:8080/clip', {
         method: 'POST',
         mode: 'cors',
         headers: { 'Content-Type': 'application/json' },
         body: JSON.stringify({ company: 'CORS Test' })
       });
       ```
    3. If the fetch fails due to mixed-content or CORS policies, trigger the fallback window:
       ```javascript
       window.open('http://127.0.0.1:8080/clip?company=PopupTest', '_blank');
       ```
*   **Expected Behavior:** 
    *   The `fetch` request should succeed if Private Network Access headers are correctly handled.
    *   If fallback is triggered, verify if the browser blocks the popup and if the bookmarklet alerts the user with helper instructions.

---

### TC-03: DOM Scraper Fragility & Site Suffix Parsing
*   **Goal:** Check data extraction resilience against weird job titles and different page structures.
*   **Preconditions:** LeGaJ clip server is running.
*   **Execution Steps:**
    1. Send a payload where the company name is empty but the link is present:
       ```powershell
       Invoke-RestMethod -Uri "http://127.0.0.1:8080/clip" -Method Post -ContentType "application/json" -Body '{"role": "Engineer", "link": "https://careers.google.com/jobs/123"}'
       ```
    2. Send a payload with site suffixes in the title:
       ```powershell
       Invoke-RestMethod -Uri "http://127.0.0.1:8080/clip" -Method Post -ContentType "application/json" -Body '{"company": "Google", "role": "Senior Engineer | Jobs at Google | LinkedIn", "link": "https://example.com"}'
       ```
*   **Expected Behavior:**
    *   The backend should successfully parse the domain name (`google.com` or `Google`) from the link if the company is missing.
    *   The backend should sanitize suffixes like `| LinkedIn` or `- Indeed` from the role title.

---

### TC-04: Security Verification (Unauthenticated Flooding)
*   **Goal:** Verify if the clip server handles excessive/malicious payloads safely.
*   **Preconditions:** LeGaJ clip server is running.
*   **Execution Steps:**
    1. Send a very large payload (e.g., 50MB description string) to the `/clip` endpoint.
    2. Send a flood of 200 consecutive POST requests in under 5 seconds.
    3. Check if LeGaJ crashes, runs out of memory, or freezes.
*   **Expected Behavior:** The server should reject excessively large payloads (limit body size to < 100KB) and implement basic rate limiting or token checks to prevent UI freezing.

---

### TC-05: Headless / CLI Execution Safety
*   **Goal:** Verify that the server doesn't crash when running without a GUI (CLI mode).
*   **Preconditions:** None.
*   **Execution Steps:**
    1. Run a LeGaJ command that doesn't start the GUI (e.g., `legaj search-jobs -keywords "QA" -location "Remote"`).
    2. Send a POST request to port `8080` (if listening) or check if the listener is inactive during CLI runs.
*   **Expected Behavior:**
    *   The clip server should **not** run in CLI-only mode.
    *   If the clip handler executes, it must check `if state.ClipInboxBox == nil { return }` or similar safety guards to prevent nil pointer crashes.
