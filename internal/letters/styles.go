package letters

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// Style is a way to open a letter. Rotating them keeps ten letters
// generated in one go from all starting the same way (R21).
type Style struct {
	ID          string
	Instruction string
}

// Styles are the four openings, rotated in this order.
var Styles = []Style{
	{ID: "achievement", Instruction: "Open with one concrete achievement from the profile that matters for this role."},
	{ID: "mission", Instruction: "Open by connecting something specific in the job description to what the candidate cares about."},
	{ID: "story", Instruction: "Open with a one-sentence moment from the candidate's experience that led them to this kind of work."},
	{ID: "direct", Instruction: "Open with a direct, confident statement of the role and the strongest reason the candidate fits."},
}

// StyleInstruction returns the prompt text for a style ID.
func StyleInstruction(id string) string {
	for _, s := range Styles {
		if s.ID == id {
			return s.Instruction
		}
	}
	return Styles[0].Instruction
}

// styleKey stores the rotation position in the settings table.
const styleKey = "letters.nextStyle"

// takeStyle returns the next style and advances the rotation, inside the
// caller's write transaction, so concurrent drafts get different styles.
func takeStyle(ctx context.Context, tx *sql.Tx) (Style, error) {
	var raw string
	n := 0
	err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, styleKey).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Style{}, err
	default:
		if v, convErr := strconv.Atoi(raw); convErr == nil && v >= 0 {
			n = v
		}
	}
	style := Styles[n%len(Styles)]
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, styleKey, strconv.Itoa((n+1)%len(Styles)))
	return style, err
}
