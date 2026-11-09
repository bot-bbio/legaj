-- 0002: the domain model (plan item 8).
-- Times are UTC text in store.FormatTime's fixed-width format, so they sort
-- correctly as strings. Booleans are 0/1 integers.

-- The user's profile: one JSON document, validated in Go (internal/profile).
-- Kept whole so saving never drops fields (V1 bugs A2-A5).
CREATE TABLE profile (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    data       TEXT    NOT NULL CHECK (json_valid(data)),
    updated_at TEXT    NOT NULL
) STRICT;

-- Every uploaded resume, kept unchanged on disk (R8, R10).
CREATE TABLE resume_files (
    id          INTEGER PRIMARY KEY,
    file_name   TEXT    NOT NULL,           -- the user's original file name
    stored_path TEXT    NOT NULL UNIQUE,    -- relative to the data folder
    mime_type   TEXT    NOT NULL,
    sha256      TEXT    NOT NULL,
    size_bytes  INTEGER NOT NULL CHECK (size_bytes >= 0),
    uploaded_at TEXT    NOT NULL
) STRICT;

-- One job the user is pursuing. id is a surrogate key (C16): rows are
-- never addressed by position or by company + role (V1 bugs U4, A19).
CREATE TABLE applications (
    id             INTEGER PRIMARY KEY,
    company        TEXT    NOT NULL DEFAULT '',   -- '' = "Needs company" (R19)
    role           TEXT    NOT NULL DEFAULT '',
    location       TEXT    NOT NULL DEFAULT '',
    url            TEXT    NOT NULL DEFAULT '',   -- as clipped or pasted
    canonical_url  TEXT    NOT NULL DEFAULT '',   -- for dedupe (R15); '' = none
    description    TEXT    NOT NULL DEFAULT '',   -- the job description (R16)
    source         TEXT    NOT NULL CHECK (source IN ('clip', 'paste')),
    stage          TEXT    NOT NULL DEFAULT 'lead'
                   CHECK (stage IN ('lead', 'ready', 'applied', 'interviewing', 'offer', 'rejected', 'withdrawn')),
    applied_at     TEXT,
    resume_file_id INTEGER REFERENCES resume_files(id) ON DELETE RESTRICT,  -- version used to apply (R10)
    notes          TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL,
    deleted_at     TEXT                            -- soft delete, purged after the undo window (R13)
) STRICT;

-- Dedupe and uniqueness ignore soft-deleted rows (R13), so re-clipping a
-- deleted job creates a new row.
CREATE UNIQUE INDEX applications_canonical_url_live
    ON applications (canonical_url)
    WHERE deleted_at IS NULL AND canonical_url <> '';
CREATE INDEX applications_live_stage   ON applications (stage)      WHERE deleted_at IS NULL;
CREATE INDEX applications_live_created ON applications (created_at) WHERE deleted_at IS NULL;

-- Every stage change, including automatic ones (R11).
CREATE TABLE stage_events (
    id             INTEGER PRIMARY KEY,
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    from_stage     TEXT,                          -- NULL when the row is created
    to_stage       TEXT    NOT NULL,
    cause          TEXT    NOT NULL CHECK (cause IN ('user', 'auto')),
    reason         TEXT    NOT NULL DEFAULT '',   -- e.g. 'clip', 'materials_approved', 'round_added'
    at             TEXT    NOT NULL
) STRICT;
CREATE INDEX stage_events_app ON stage_events (application_id, at);

-- Cover letters: drafts, then approved versions with a PDF (R21, R22).
CREATE TABLE documents (
    id             INTEGER PRIMARY KEY,
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL CHECK (kind IN ('letter')),
    status         TEXT    NOT NULL CHECK (status IN ('draft', 'approved')),
    version        INTEGER CHECK (version IS NULL OR version >= 1),  -- set on approval
    body           TEXT    NOT NULL CHECK (json_valid(body)),       -- paragraphs, why options, style
    file_path      TEXT,                                            -- PDF, relative to the data folder
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL,
    approved_at    TEXT
) STRICT;
CREATE UNIQUE INDEX documents_version ON documents (application_id, kind, version) WHERE version IS NOT NULL;
CREATE INDEX documents_app ON documents (application_id);

-- Form answers and the answer bank (R25). Banked answers outlive their
-- application, so the link is cleared rather than cascaded.
CREATE TABLE answers (
    id             INTEGER PRIMARY KEY,
    application_id INTEGER REFERENCES applications(id) ON DELETE SET NULL,
    question       TEXT    NOT NULL,
    answer         TEXT    NOT NULL DEFAULT '',
    options        TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(options)),  -- generated alternatives
    status         TEXT    NOT NULL CHECK (status IN ('draft', 'approved')),
    in_bank        INTEGER NOT NULL DEFAULT 0 CHECK (in_bank IN (0, 1)),
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
) STRICT;
CREATE INDEX answers_app ON answers (application_id);
CREATE INDEX answers_bank ON answers (in_bank) WHERE in_bank = 1;

-- Interview rounds (R30): a UTC instant plus the IANA zone it was entered in.
CREATE TABLE rounds (
    id             INTEGER PRIMARY KEY,
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    starts_at      TEXT    NOT NULL,
    zone           TEXT    NOT NULL,
    duration_min   INTEGER NOT NULL DEFAULT 60 CHECK (duration_min > 0),
    kind           TEXT    NOT NULL DEFAULT 'other',
    location       TEXT    NOT NULL DEFAULT '',
    link           TEXT    NOT NULL DEFAULT '',
    interviewers   TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(interviewers)),
    prep_notes     TEXT    NOT NULL DEFAULT '',
    post_notes     TEXT    NOT NULL DEFAULT '',
    outcome        TEXT    NOT NULL DEFAULT '',
    questions      TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(questions)),  -- questions to ask (R32)
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
) STRICT;
CREATE INDEX rounds_app ON rounds (application_id, starts_at);

-- Follow-ups the user drafted, sent or skipped (R27-R29). Due dates are
-- computed in Go from stage events and rounds.
CREATE TABLE followups (
    id             INTEGER PRIMARY KEY,
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    round_id       INTEGER REFERENCES rounds(id) ON DELETE CASCADE,  -- set for thank-you notes
    kind           TEXT    NOT NULL CHECK (kind IN ('applied_nudge', 'thank_you')),
    status         TEXT    NOT NULL CHECK (status IN ('draft', 'sent', 'skipped')),
    subject        TEXT    NOT NULL DEFAULT '',
    body           TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    resolved_at    TEXT
) STRICT;
CREATE INDEX followups_app ON followups (application_id);

-- App settings (R37): one JSON value per key. Defaults live in Go.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL CHECK (json_valid(value))
) STRICT;
