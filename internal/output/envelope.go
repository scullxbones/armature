package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

// Envelope is the Agent Output Contract object {count, <payload>[], help[]}.
// The payload key is a command-declared plural name, never the literal key "payload".
type Envelope struct {
	payloadKey string
	items      any
	adjuncts   map[string]json.RawMessage
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
	// N6: an empty result is still a result, and help is where its reason
	// lives. Without this, a caller passing nil help emits {"count":0,...,
	// "help":[]}, which says nothing about why the result is empty.
	if rv.Len() == 0 && len(help) == 0 {
		return nil, fmt.Errorf("envelope with zero items must carry help naming why the result is empty")
	}
	helpCopy := make([]string, len(help))
	copy(helpCopy, help)
	return &Envelope{payloadKey: payloadKey, items: items, help: helpCopy}, nil
}

// AddAdjunct adds a named result field between the payload and trailing help.
func (e *Envelope) AddAdjunct(key string, value any) error {
	if key == "" || key == "count" || key == e.payloadKey || key == "help" || key == "payload" {
		return fmt.Errorf("envelope adjunct key %q conflicts with a reserved member", key)
	}
	if _, exists := e.adjuncts[key]; exists {
		return fmt.Errorf("envelope adjunct key %q is already present", key)
	}
	data, err := marshalVerbatim(value)
	if err != nil {
		return fmt.Errorf("marshal envelope adjunct %q: %w", key, err)
	}
	if e.adjuncts == nil {
		e.adjuncts = make(map[string]json.RawMessage)
	}
	e.adjuncts[key] = data
	return nil
}

// MarshalJSON emits {count, <payloadKey>[], help[]} with count equal to payload length.
// Members are written in that order because help trails the payload (N5.1); a
// map would be re-sorted lexicographically by encoding/json, putting help ahead
// of payload keys such as issues, workers, and worktrees.
//
// Prefer WriteEnvelope for output. json.Marshal re-escapes a Marshaler's bytes
// for HTML, which would turn contract text like "arm show <id>" back into
// its escaped form.
func (e *Envelope) MarshalJSON() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	if e.items == nil {
		return nil, fmt.Errorf("envelope payload must be an array")
	}
	n := reflect.ValueOf(e.items).Len()
	payload, err := marshalVerbatim(e.items)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope payload: %w", err)
	}
	if string(payload) == "null" {
		payload = []byte("[]")
	}
	key, err := marshalVerbatim(e.payloadKey)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope payload key: %w", err)
	}
	helpJSON, err := marshalVerbatim(e.help)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope help: %w", err)
	}
	if string(helpJSON) == "null" {
		helpJSON = []byte("[]")
	}

	var buf bytes.Buffer
	buf.WriteString(`{"count":`)
	buf.WriteString(strconv.Itoa(n))
	buf.WriteByte(',')
	buf.Write(key)
	buf.WriteByte(':')
	buf.Write(payload)
	adjunctKeys := make([]string, 0, len(e.adjuncts))
	for adjunctKey := range e.adjuncts {
		adjunctKeys = append(adjunctKeys, adjunctKey)
	}
	sort.Strings(adjunctKeys)
	for _, adjunctKey := range adjunctKeys {
		keyJSON, err := marshalVerbatim(adjunctKey)
		if err != nil {
			return nil, fmt.Errorf("marshal envelope adjunct key %q: %w", adjunctKey, err)
		}
		buf.WriteByte(',')
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(e.adjuncts[adjunctKey])
	}
	buf.WriteString(`,"help":`)
	buf.Write(helpJSON)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalVerbatim marshals v without encoding/json's HTML escaping. Agents read
// this output as text, and the contract's own examples spell "<id>" literally.
func marshalVerbatim(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode appends a newline; the envelope supplies its own terminator.
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// WriteEnvelope writes one JSON envelope object and a terminating newline to w.
// Callers pass stdout; stderr is not a results channel.
func WriteEnvelope(w io.Writer, env *Envelope) error {
	if env == nil {
		return fmt.Errorf("nil envelope")
	}
	data, err := env.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
