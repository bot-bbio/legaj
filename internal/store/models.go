package store

import (
	"fmt"
	"time"
)

// Stage is an application's position in the pipeline (R11). The DB's CHECK
// constraint lists the same values; internal/pipeline owns the transitions.
type Stage string

// Pipeline stages, in display order.
const (
	StageLead         Stage = "lead"
	StageReady        Stage = "ready"
	StageApplied      Stage = "applied"
	StageInterviewing Stage = "interviewing"
	StageOffer        Stage = "offer"
	StageRejected     Stage = "rejected"
	StageWithdrawn    Stage = "withdrawn"
)

// Stages lists every stage in display order.
var Stages = []Stage{StageLead, StageReady, StageApplied, StageInterviewing, StageOffer, StageRejected, StageWithdrawn}

// Valid reports whether s is a known stage.
func (s Stage) Valid() bool {
	for _, k := range Stages {
		if s == k {
			return true
		}
	}
	return false
}

// Source says how an application entered LeGaJ.
type Source string

// Application sources.
const (
	SourceClip  Source = "clip"
	SourcePaste Source = "paste"
)

// Cause says who triggered a stage change (R11).
type Cause string

// Stage-change causes.
const (
	CauseUser Cause = "user"
	CauseAuto Cause = "auto"
)

// Application is a row of the applications table.
type Application struct {
	ID           int64      `json:"id"`
	Company      string     `json:"company"`
	Role         string     `json:"role"`
	Location     string     `json:"location"`
	URL          string     `json:"url"`
	CanonicalURL string     `json:"canonicalUrl"`
	Description  string     `json:"description"`
	Source       Source     `json:"source"`
	Stage        Stage      `json:"stage"`
	AppliedAt    *time.Time `json:"appliedAt"`
	ResumeFileID *int64     `json:"resumeFileId"`
	Notes        string     `json:"notes"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// StageEvent is a row of the stage_events table.
type StageEvent struct {
	ID            int64     `json:"id"`
	ApplicationID int64     `json:"applicationId"`
	From          *Stage    `json:"from"`
	To            Stage     `json:"to"`
	Cause         Cause     `json:"cause"`
	Reason        string    `json:"reason"`
	At            time.Time `json:"at"`
}

// NewestResumeIDQuery selects the current resume: the most recent upload.
// The one definition shared by the state machine, Open resume and the
// resume service, so "current" can't drift between them.
const NewestResumeIDQuery = `SELECT id FROM resume_files ORDER BY uploaded_at DESC, id DESC LIMIT 1`

// timeLayout is fixed width (always 3 fractional digits, always Z), so
// stored times sort correctly as text.
const timeLayout = "2006-01-02T15:04:05.000Z"

// FormatTime renders t in UTC for storage.
func FormatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

// ParseTime reads a stored time back as UTC.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("stored time %q: %w", s, err)
	}
	return t.UTC(), nil
}
