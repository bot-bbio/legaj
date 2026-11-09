package ai_test

import (
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const answerSchema = `{
  "type": "object",
  "properties": {
    "answers": {
      "type": "array",
      "minItems": 3,
      "maxItems": 3,
      "items": {"type": "string", "minLength": 1, "maxLength": 40}
    }
  },
  "required": ["answers"],
  "additionalProperties": false
}`

func TestR5_SchemaInvalidRejected(t *testing.T) {
	bad := map[string]string{
		"not json":                         `Sure! Here are three answers: ...`,
		"missing field":                    `{}`,
		"wrong type":                       `{"answers": "one"}`,
		"too few":                          `{"answers": ["a", "b"]}`,
		"too long (client-side maxLength)": `{"answers": ["a", "b", "` + strings.Repeat("x", 41) + `"]}`,
		"extra field":                      `{"answers": ["a", "b", "c"], "note": "x"}`,
		"trailing garbage":                 `{"answers": ["a", "b", "c"]} and more`,
	}
	for name, text := range bad {
		t.Run(name, func(t *testing.T) {
			err := ai.ValidateJSON([]byte(answerSchema), text)
			if err == nil {
				t.Fatal("accepted")
			}
			if apperr.CodeOf(err) != apperr.CodeAIInvalidOutput {
				t.Fatalf("code = %q", apperr.CodeOf(err))
			}
		})
	}
}

func TestR5_SchemaValidAccepted(t *testing.T) {
	if err := ai.ValidateJSON([]byte(answerSchema), `{"answers": ["a", "b", "c"]}`); err != nil {
		t.Fatal(err)
	}
}

func TestR5_BrokenSchemaIsAnError(t *testing.T) {
	if err := ai.ValidateJSON([]byte(`{"type": 5}`), `{}`); err == nil {
		t.Fatal("a broken schema must not validate anything")
	}
}
