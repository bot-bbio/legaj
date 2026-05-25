# LeGaJ Clip Tool Implementation Checklist

A roadmap of actionable tasks to fix potential bugs, security weaknesses, and reliability issues inside the LeGaJ Clipper tool.

---

## 🛠️ Task Checklist

### 1. Robust Port Management
- [ ] **Dynamic Port Allocation**
  - Implement a fallback loop checking ports `8080`, `8081`, `8082`, etc., until an open port is bound.
- [ ] **Dynamic Bookmarklet Generation**
  - Update the bookmarklet generator function [buildFileManagerTab](gui.go#L4098) to dynamically embed the active port number into the bookmarklet script.
- [ ] **App Port Registry**
  - Save the active port to a temporary file (e.g., `references/.clip_port`) so scripts and external tools can read the current active port.

### 2. HTTPS & Mixed Content Workarounds
- [ ] **Browser Extension Option**
  - Transition from a pure bookmarklet to a minimal chrome/edge extension (which bypasses local mixed-content restrictions on HTTPS sites).
- [ ] **Enhanced Popup Instructions**
  - Improve popup blocker warning messages in [gui.go](gui.go#L2719) to guide users on how to allow popups for major job sites.

### 3. Scraper Resilience & Title Sanitization
- [ ] **Title Sanitizer Regex**
  - Implement regex patterns in the `/clip` handler to strip off common job portal brand suffix tags:
    - `\s*[-|•]\s*(LinkedIn|Indeed|ZipRecruiter|Glassdoor|Google).*`
- [ ] **Hostname Domain Parser**
  - Implement a utility function to extract clean company names from raw URL hostnames when the scraped title is empty.

### 4. API Security & Rate Limiting
- [ ] **Local Auth Token Verification**
  - Generate a secure cryptographically random API token on application start.
  - Require the header `X-LeGaJ-Token` (or query param `token`) for all incoming requests.
- [ ] **Max Body Size Guard**
  - Wrap incoming request readers with `http.MaxBytesReader` to limit post bodies to `150KB` max to prevent out-of-memory crashes.
- [ ] **Rate Limiting Middleware**
  - Restrict the endpoint to a maximum of 5 requests per 10 seconds per IP address.

### 5. Headless & CLI Execution Safety
- [ ] **Conditional Clip Server Launch**
  - Ensure `startClipServer()` is only triggered when running in GUI mode (never during command line runs like `search-jobs` or `parse-resume`).
- [ ] **UI Reference Safety Checks**
  - Add explicit nil checks in the `/clip` handler closure:
    ```go
    if state == nil || state.ClipInboxBox == nil {
        return
    }
    ```
