package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// invalidOutput is the user-facing error for model output that doesn't
// match what was asked for.
func invalidOutput(cause error) error {
	return apperr.Wrap(cause, apperr.CodeAIInvalidOutput, "Gemini's answer wasn't in the expected format. Try again.")
}

var (
	schemaMu    sync.Mutex
	schemaCache = map[string]*jsonschema.Schema{}
)

func compile(schema []byte) (*jsonschema.Schema, error) {
	key := string(schema)
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s, ok := schemaCache[key]; ok {
		return s, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, fmt.Errorf("schema is not JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	const loc = "mem://legaj/schema.json"
	if err := c.AddResource(loc, doc); err != nil {
		return nil, err
	}
	s, err := c.Compile(loc)
	if err != nil {
		return nil, fmt.Errorf("schema does not compile: %w", err)
	}
	schemaCache[key] = s
	return s, nil
}

// ValidateJSON checks that text is exactly one JSON value matching schema,
// including the keywords Gemini ignores server-side (minLength, maxLength,
// …). The returned error is an apperr with CodeAIInvalidOutput, and its
// cause never quotes the output.
func ValidateJSON(schema []byte, text string) error {
	s, err := compile(schema)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber() // the validator expects json.Number
	var v any
	if err := dec.Decode(&v); err != nil {
		return invalidOutput(errors.New("model output is not JSON"))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return invalidOutput(errors.New("model output has data after the JSON value"))
	}
	if err := s.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return invalidOutput(fmt.Errorf("model output failed schema validation at /%s", strings.Join(ve.InstanceLocation, "/")))
		}
		return invalidOutput(errors.New("model output failed schema validation"))
	}
	return nil
}
