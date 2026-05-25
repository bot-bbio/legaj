package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractCompanyNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://careers.google.com/jobs/123", "Google"},
		{"https://indeed.com/viewjob?id=123", "Indeed"},
		{"https://jobs.lever.co/companyname/123", "Lever"},
		{"https://www.apple.com/jobs", "Apple"},
		{"invalid-url", "Web"},
	}

	for _, tc := range tests {
		result := extractCompanyNameFromURL(tc.url)
		if result != tc.expected {
			t.Errorf("For URL %q, expected %q, got %q", tc.url, tc.expected, result)
		}
	}
}

func TestExtractCompanyFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"Senior Engineer at Google", "Google"},
		{"Google - Senior Engineer", "Google"},
		{"Senior Engineer | Apple", "Apple"},
		{"Software Engineer • Microsoft", "Microsoft"},
		{"Just a Role Title", ""},
	}

	for _, tc := range tests {
		result := extractCompanyFromTitle(tc.title)
		if result != tc.expected {
			t.Errorf("For title %q, expected company %q, got %q", tc.title, tc.expected, result)
		}
	}
}

func TestTitleSanitizerRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Senior Engineer | LinkedIn", "Senior Engineer"},
		{"Senior Engineer | Jobs at Google | LinkedIn", "Senior Engineer | Jobs at Google"},
		{"Software Developer - Indeed", "Software Developer"},
		{"QA Lead • Glassdoor", "QA Lead"},
		{"Product Manager - Google Careers", "Product Manager"},
	}

	for _, tc := range tests {
		result := titleSanitizerRegex.ReplaceAllString(tc.input, "")
		result = strings.TrimSpace(result)
		if result != tc.expected {
			t.Errorf("For input %q, expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestPortConflictAndFallback(t *testing.T) {
	// Bind to port 8080 to simulate a conflict
	l8080, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Skip("Could not bind to 8080 for testing, port might be in use by another running instance of LeGaJ or service:", err)
	}
	defer l8080.Close()

	// Call initClipServer
	initClipServer()

	// Verify port is bound to something other than 8080
	if activePort == 8080 {
		t.Error("Expected activePort to not be 8080 due to conflict")
	}

	// Verify clipListener is not nil
	if clipListener == nil {
		t.Error("Expected clipListener to not be nil")
	}
	clipListener.Close() // clean up
}

func TestClipHandlerSecurityAndLimits(t *testing.T) {
	// Set up global token for test
	clipAuthToken = "test_token_xyz"
	isGUIMode = false // simulated CLI/headless mode

	// Create test handler from clipMux
	var err error
	clipListener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer clipListener.Close()

	clipServerStarted = false // reset for test
	startClipServer()

	// 1. Test missing token
	req, _ := http.NewRequest("POST", "/clip", strings.NewReader(`{"company":"Test"}`))
	rr := httptest.NewRecorder()
	clipMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for missing token, got %d", rr.Code)
	}

	// 2. Test valid token but huge body (> 100KB)
	largeBody := make([]byte, 105*1024)
	for i := range largeBody {
		largeBody[i] = 'A'
	}
	req, _ = http.NewRequest("POST", "/clip?token=test_token_xyz", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	clipMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for excessively large payload, got %d", rr.Code)
	}

	// 3. Test valid token and valid body in headless mode
	req, _ = http.NewRequest("POST", "/clip?token=test_token_xyz", strings.NewReader(`{"company":"Google","role":"Engineer"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	clipMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for valid request, got %d", rr.Code)
	}

	// 4. Test rate limiting
	// Send 5 more requests (total of 6 in short period)
	rateLimited := false
	for i := 0; i < 10; i++ {
		req, _ = http.NewRequest("POST", "/clip?token=test_token_xyz", strings.NewReader(`{"company":"Google","role":"Engineer"}`))
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		clipMux.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}
	if !rateLimited {
		t.Error("Expected rate limiter to trigger 429 Too Many Requests")
	}
}
