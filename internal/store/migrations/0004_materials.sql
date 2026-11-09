-- 0004: each application's materials choice (R26): what must be approved
-- before a Lead auto-moves to Ready. New rows take the Settings default;
-- '' means nothing chosen, and such a row never advances on its own.
ALTER TABLE applications ADD COLUMN materials TEXT NOT NULL DEFAULT 'letter'
    CHECK (materials IN ('', 'letter', 'answers', 'both'));
