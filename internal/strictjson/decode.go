// Package strictjson provides the shared decoding policy for versioned JSON artifacts.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Decode decodes exactly one JSON value, rejecting any trailing JSON data.
// Versioned artifacts use this policy so a permissive decoder cannot silently
// accept a malformed or stale protocol payload composed of multiple
// concatenated values. Unknown object fields are intentionally allowed: the
// canonical validators (docs/schemas/*.schema.json) do not set
// additionalProperties: false, so a schema-valid plan, review bundle, or
// conformance assessment may legitimately carry extension/metadata fields.
// Rejecting them here would fail artifacts the published contract accepts.
func Decode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(value); err != nil {
		return err
	}

	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("unexpected trailing JSON value")
	}
	return fmt.Errorf("unexpected trailing JSON data: %w", err)
}
