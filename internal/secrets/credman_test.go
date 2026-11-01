//go:build windows && credman

// The real Windows Credential Manager round trip. It writes a throwaway
// credential, so it only runs on request, as part of the manual smoke test:
//
//	$env:LEGAJ_HOME = "$env:TEMP\legaj-smoke"; go test -tags credman ./internal/secrets/
package secrets_test

import (
	"errors"
	"testing"

	"github.com/bot-bbio/legaj/internal/secrets"
)

func TestR4_CredentialManagerRoundTrip(t *testing.T) {
	k := secrets.NewKeyringFor("LeGaJ-test", "credman-roundtrip")
	t.Cleanup(func() { _ = k.Delete() })

	if err := k.Set(sentinelKey); err != nil {
		t.Fatal(err)
	}
	got, err := k.Get()
	if err != nil || got != sentinelKey {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := k.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get(); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("after Delete: %v, want ErrNotFound", err)
	}
}
