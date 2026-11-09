package apperr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const secretCause = "open C:\\Users\\lukasz\\AppData\\Local\\LeGaJ\\legaj.db: key AIzaSy-SENTINEL; goroutine 1 [running]:\nmain.main()"

func TestR34_UserErrorHasNoCause(t *testing.T) {
	cause := errors.New(secretCause)
	err := apperr.Wrap(cause, apperr.CodeStorage, "LeGaJ couldn't save your changes. Try again.")

	views := map[string]string{
		"Error()": err.Error(),
		"%v":      fmt.Sprintf("%v", err),
		"%+v":     fmt.Sprintf("%+v", err),
		"%s":      fmt.Sprintf("%s", err),
		"%#v":     fmt.Sprintf("%#v", err),
	}
	js, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatal(jerr)
	}
	views["json"] = string(js)
	views["UserMessage"] = apperr.UserMessage(err)

	for name, v := range views {
		for _, leak := range []string{"SENTINEL", "lukasz", "goroutine", "legaj.db"} {
			if strings.Contains(v, leak) {
				t.Errorf("%s leaks %q: %s", name, leak, v)
			}
		}
	}
	if !errors.Is(err, cause) {
		t.Fatal("the cause must stay reachable for logging via errors.Is/Unwrap")
	}
	if got := apperr.CodeOf(err); got != apperr.CodeStorage {
		t.Fatalf("CodeOf = %q", got)
	}
}

func TestR34_JSONHasOnlyCodeAndMessage(t *testing.T) {
	err := apperr.Wrap(errors.New("x"), apperr.CodeAIKeyRejected, "Gemini rejected the key.")
	var m map[string]any
	js, _ := json.Marshal(err)
	if e := json.Unmarshal(js, &m); e != nil {
		t.Fatal(e)
	}
	if len(m) != 2 || m["code"] != string(apperr.CodeAIKeyRejected) || m["message"] != "Gemini rejected the key." {
		t.Fatalf("json = %s", js)
	}
}

func TestR34_PublicHidesUnknownErrors(t *testing.T) {
	cause := errors.New(secretCause)
	raw := fmt.Errorf("wrapped: %w", cause)
	pub := apperr.Public(raw)
	if strings.Contains(pub.Error(), "SENTINEL") || strings.Contains(pub.Error(), "goroutine") {
		t.Fatalf("Public leaked the cause: %s", pub.Error())
	}
	if apperr.CodeOf(pub) != apperr.CodeInternal {
		t.Fatalf("code = %q, want internal", apperr.CodeOf(pub))
	}
	if !errors.Is(pub, cause) {
		t.Fatal("Public must keep the cause reachable for logging")
	}
	if apperr.Public(nil) != nil {
		t.Fatal("Public(nil) must be nil")
	}

	// An app error passes through unchanged, wherever it is in the chain.
	inner := apperr.New(apperr.CodeNotFound, "That application no longer exists.")
	if got := apperr.Public(fmt.Errorf("ctx: %w", inner)); got.Error() != inner.Error() {
		t.Fatalf("Public changed an app error's message: %q", got.Error())
	}
}
