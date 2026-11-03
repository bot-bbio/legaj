-- 0005: the address a follow-up draft goes to (R29). Optional: web mail
-- clients can open a compose window without one.
ALTER TABLE followups ADD COLUMN recipient TEXT NOT NULL DEFAULT '';
