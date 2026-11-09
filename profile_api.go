package main

import (
	"os"
	"path/filepath"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/resume"
)

// ProfileAPI is the Profile screen's binding (R9, R10).
type ProfileAPI struct {
	app *App
}

// GetProfile returns the saved profile (empty lists, never null).
func (p *ProfileAPI) GetProfile() (profile.Profile, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return profile.Profile{}, err
	}
	out, err := svc.Profile.Get(p.app.ctx)
	return out, p.app.public("ProfileAPI.GetProfile", err)
}

// SaveProfile validates and stores the whole profile, returning what was saved.
func (p *ProfileAPI) SaveProfile(in profile.Profile) (profile.Profile, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return profile.Profile{}, err
	}
	out, err := svc.Profile.Save(p.app.ctx, in)
	if err != nil {
		return profile.Profile{}, p.app.public("ProfileAPI.SaveProfile", err)
	}
	return out, nil
}

// CurrentResume returns the resume new applications will use. ID 0 means
// none has been uploaded yet; that isn't an error.
func (p *ProfileAPI) CurrentResume() (ResumeFileView, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return ResumeFileView{}, err
	}
	info, err := svc.Resume.Current(p.app.ctx)
	if apperr.CodeOf(err) == apperr.CodeNotFound {
		return ResumeFileView{}, nil
	}
	if err != nil {
		return ResumeFileView{}, p.app.public("ProfileAPI.CurrentResume", err)
	}
	return toFileView(info), nil
}

// UploadNewResume stores a new version of the resume (R10). Applications
// already marked Applied keep the version they were sent with. The profile
// is not changed: re-reading it is the wizard's job, with review.
func (p *ProfileAPI) UploadNewResume() (ResumeImport, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return ResumeImport{}, err
	}
	info, cancelled, err := p.app.pickAndStoreResume(svc.Resume)
	if err != nil || cancelled {
		return ResumeImport{Cancelled: cancelled}, p.app.public("ProfileAPI.UploadNewResume", err)
	}
	return ResumeImport{File: toFileView(info)}, nil
}

// OpenCurrentResume opens the current resume in the default viewer (R33).
func (p *ProfileAPI) OpenCurrentResume() error {
	c, svc, err := p.app.services()
	if err != nil {
		return err
	}
	info, err := svc.Resume.Current(p.app.ctx)
	if err != nil {
		return p.app.public("ProfileAPI.OpenCurrentResume", err)
	}
	path, err := svc.Resume.Path(info)
	if err != nil {
		return p.app.public("ProfileAPI.OpenCurrentResume", err)
	}
	return p.app.public("ProfileAPI.OpenCurrentResume", c.Opener.OpenFile(path))
}

// pickAndStoreResume shows the file dialog, reads the chosen file in Go
// (its bytes never cross the JavaScript bridge) and stores it unchanged.
func (a *App) pickAndStoreResume(r *resume.Service) (info resume.FileInfo, cancelled bool, err error) {
	path, err := a.pickFile(a.ctx)
	if err != nil {
		return resume.FileInfo{}, false, err
	}
	if path == "" {
		return resume.FileInfo{}, true, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return resume.FileInfo{}, false, apperr.Wrap(err, apperr.CodeInvalid, "LeGaJ couldn't open that file.")
	}
	if fi.Size() > resume.MaxFileBytes {
		return resume.FileInfo{}, false, apperr.New(apperr.CodeInvalid, "That file is too large. Resumes up to 10 MB are supported.")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return resume.FileInfo{}, false, apperr.Wrap(err, apperr.CodeInvalid, "LeGaJ couldn't open that file.")
	}
	info, err = r.Store(a.ctx, filepath.Base(path), data)
	return info, false, err
}
