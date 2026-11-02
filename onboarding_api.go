package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/onboarding"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/resume"
)

// OnboardingAPI is the first-run wizard's binding (R7).
type OnboardingAPI struct {
	app *App
}

// ResumeFileView describes a stored resume for the UI.
type ResumeFileView struct {
	ID         int64  `json:"id"`
	FileName   string `json:"fileName"`
	SizeBytes  int64  `json:"sizeBytes"`
	UploadedAt string `json:"uploadedAt"` // RFC 3339, UTC
}

// ResumeImport is the result of choosing and reading a resume.
type ResumeImport struct {
	// Cancelled means the user closed the file dialog.
	Cancelled bool           `json:"cancelled"`
	File      ResumeFileView `json:"file"`
	// Draft is the parsed profile for review; nil when parsing failed.
	Draft *profile.Profile `json:"draft"`
	// ParseError is a user-safe reason the parse failed (retry or fill in by hand).
	ParseError string `json:"parseError"`
}

func toFileView(f resume.FileInfo) ResumeFileView {
	return ResumeFileView{ID: f.ID, FileName: f.FileName, SizeBytes: f.SizeBytes, UploadedAt: f.UploadedAt.UTC().Format("2006-01-02T15:04:05Z")}
}

// pickResumeFile shows Windows' open-file dialog. The file is read in Go,
// so its bytes never cross the JavaScript bridge.
func pickResumeFile(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:   "Choose your resume",
		Filters: []runtime.FileFilter{{DisplayName: "Resume (PDF or Word)", Pattern: "*.pdf;*.docx"}},
	})
}

// OnboardingState returns the wizard and checklist state.
func (o *OnboardingAPI) OnboardingState() (onboarding.State, error) {
	_, svc, err := o.app.services()
	if err != nil {
		return onboarding.State{}, err
	}
	st, err := svc.Onboarding.State(o.app.ctx)
	return st, apperr.Public(err)
}

// SaveKey tests and saves a pasted API key.
func (o *OnboardingAPI) SaveKey(key string) (onboarding.KeyResult, error) {
	_, svc, err := o.app.services()
	if err != nil {
		return onboarding.KeyResult{}, err
	}
	res, err := svc.Onboarding.SaveKey(o.app.ctx, key)
	return res, apperr.Public(err)
}

// UploadResume lets the user pick a resume, stores it unchanged, and parses
// it into a draft for review.
func (o *OnboardingAPI) UploadResume() (ResumeImport, error) {
	_, svc, err := o.app.services()
	if err != nil {
		return ResumeImport{}, err
	}
	path, err := o.app.pickFile(o.app.ctx)
	if err != nil {
		return ResumeImport{}, apperr.Public(err)
	}
	if path == "" {
		return ResumeImport{Cancelled: true}, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return ResumeImport{}, apperr.Wrap(err, apperr.CodeInvalid, "LeGaJ couldn't open that file.")
	}
	if fi.Size() > resume.MaxFileBytes {
		return ResumeImport{}, apperr.New(apperr.CodeInvalid, "That file is too large. Resumes up to 10 MB are supported.")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ResumeImport{}, apperr.Wrap(err, apperr.CodeInvalid, "LeGaJ couldn't open that file.")
	}
	info, err := svc.Resume.Store(o.app.ctx, filepath.Base(path), data)
	if err != nil {
		return ResumeImport{}, apperr.Public(err)
	}
	return o.parse(svc.Resume, info)
}

// ParseResume retries parsing an already stored resume.
func (o *OnboardingAPI) ParseResume(fileID int64) (ResumeImport, error) {
	_, svc, err := o.app.services()
	if err != nil {
		return ResumeImport{}, err
	}
	info, err := svc.Resume.Get(o.app.ctx, fileID)
	if err != nil {
		return ResumeImport{}, apperr.Public(err)
	}
	return o.parse(svc.Resume, info)
}

func (o *OnboardingAPI) parse(r *resume.Service, info resume.FileInfo) (ResumeImport, error) {
	out := ResumeImport{File: toFileView(info)}
	draft, err := r.Parse(o.app.ctx, info.ID)
	if err != nil {
		o.app.log().Warn("resume parse failed", "err", err)
		out.ParseError = apperr.UserMessage(err)
		return out, nil
	}
	out.Draft = &draft
	return out, nil
}

// FinishOnboarding records that the wizard was completed or skipped through.
func (o *OnboardingAPI) FinishOnboarding() error {
	_, svc, err := o.app.services()
	if err != nil {
		return err
	}
	return apperr.Public(svc.Onboarding.Finish(o.app.ctx))
}
