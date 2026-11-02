-- 0003: group soft-deleted rows by the delete that removed them, so Undo
-- restores exactly that batch and the purge timer removes exactly it (R13).
ALTER TABLE applications ADD COLUMN delete_token TEXT;
CREATE INDEX applications_delete_token ON applications (delete_token) WHERE delete_token IS NOT NULL;
