package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Envelope is the Agent Output Contract object {count, <payload>[], help[]}.
// The payload key is a command-declared plural name, never the literal key "payload".
type Envelope struct {
	payloadKey string
	items      any
	help       []string
}

// NewEnvelope constructs an envelope for a named payload array.
// items must be a slice or array. help is copied; a nil help becomes an empty array.
func NewEnvelope(payloadKey string, items any, help []string) (*Envelope, error) {
	if payloadKey == "" || payloadKey == "count" || payloadKey == "help" || payloadKey == "payload" {
		return nil, fmt.Errorf("envelope payload key %q is not a command-declared name", payloadKey)
	}
	if items == nil {
		return nil, fmt.Errorf("envelope payload must be an array")
	}
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("envelope payload must be an array")
	}
	// encoding/json marshals []byte, [N]byte, and aliases such as json.RawMessage
	// as a base64 string (or raw JSON), not a JSON array. Reject those so count
	// equals payload length and the payload key is always an array (N2).
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, fmt.Errorf("envelope payload must be an array")
	}
	for i, h := range help {
		if h == "" {
			return nil, fmt.Errorf("envelope help[%d] must be a non-empty string", i)
		}
	}
	helpCopy := make([]string, len(help))
	copy(helpCopy, help)
	return &Envelope{payloadKey: payloadKey, items: items, help: helpCopy}, nil
}

// MarshalJSON emits {count, <payloadKey>[], help[]} with count equal to payload length.
func (e *Envelope) MarshalJSON() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	n := reflect.ValueOf(e.items).Len()
	payload, err := json.Marshal(e.items)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope payload: %w", err)
	}
	if string(payload) == "null" {
		payload = []byte("[]")
	}
	help := e.help
	if help == nil {
		help = []string{}
	}
	return json.Marshal(map[string]any{
		"count":      n,
		e.payloadKey: json.RawMessage(payload),
		"help":       help,
	})
}

// WriteEnvelope writes one JSON envelope object and a terminating newline to w.
// Callers pass stdout; stderr is not a results channel.
func WriteEnvelope(w io.Writer, env *Envelope) error {
	if env == nil {
		return fmt.Errorf("nil envelope")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
