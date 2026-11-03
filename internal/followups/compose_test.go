package followups_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/followups"
)

func TestR29_GmailOutlookMailtoGolden(t *testing.T) {
	m := followups.Message{
		To:      "grace@example.com",
		Subject: "R&D role — follow-up",
		Body:    "Hi Łukasz,\nThanks & regards",
	}
	golden := map[followups.Client]string{
		followups.Gmail: "https://mail.google.com/mail/?view=cm&fs=1&to=grace%40example.com" +
			"&su=R%26D%20role%20%E2%80%94%20follow-up&body=Hi%20%C5%81ukasz%2C%0AThanks%20%26%20regards",
		followups.OutlookCom: "https://outlook.live.com/mail/0/deeplink/compose?to=grace%40example.com" +
			"&subject=R%26D%20role%20%E2%80%94%20follow-up&body=Hi%20%C5%81ukasz%2C%0AThanks%20%26%20regards",
		followups.Outlook365: "https://outlook.office.com/mail/deeplink/compose?to=grace%40example.com" +
			"&subject=R%26D%20role%20%E2%80%94%20follow-up&body=Hi%20%C5%81ukasz%2C%0AThanks%20%26%20regards",
		// RFC 6068: line breaks are CRLF; the address keeps its "@".
		followups.MailApp: "mailto:grace@example.com" +
			"?subject=R%26D%20role%20%E2%80%94%20follow-up&body=Hi%20%C5%81ukasz%2C%0D%0AThanks%20%26%20regards",
	}
	for c, want := range golden {
		got, err := followups.ComposeURL(c, m)
		if err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", c, got, want)
		}
	}

	// Web clients can open without a recipient; the mail app needs one.
	if u, err := followups.ComposeURL(followups.Gmail, followups.Message{Subject: "Hi", Body: "x"}); err != nil || strings.Contains(u, "to=") {
		t.Fatalf("Gmail without a recipient: %q %v", u, err)
	}
	if _, err := followups.ComposeURL(followups.MailApp, followups.Message{Subject: "Hi", Body: "x"}); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("mail app without a recipient: err = %v", err)
	}
	for _, bad := range []string{"not-an-email", "a@b.com?bcc=evil@x.com", "a@b.com, c@d.com", "a b@c.com", "<a@b.com>"} {
		if _, err := followups.ComposeURL(followups.Gmail, followups.Message{To: bad, Body: "x"}); apperr.CodeOf(err) != apperr.CodeInvalid {
			t.Errorf("recipient %q accepted: %v", bad, err)
		}
	}
	if _, err := followups.ComposeURL("carrier-pigeon", m); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("unknown client: %v", err)
	}
}

func TestR29_LongBodyWarns(t *testing.T) {
	if w := (followups.Message{Body: strings.Repeat("a", followups.LongBody)}).Warnings(); len(w) != 0 {
		t.Fatalf("2,000 characters warned: %v", w)
	}
	w := (followups.Message{Body: strings.Repeat("ł", followups.LongBody+1)}).Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "2,000") {
		t.Fatalf("2,001 characters: %v", w)
	}
}
