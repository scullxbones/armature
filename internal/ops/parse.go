package ops

import (
	"encoding/json"
	"fmt"
)

// ParseLine parses a single JSONL line in positional array format:
// [op_type, target_id, timestamp, worker_id, payload_object]
func ParseLine(line []byte) (Op, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return Op{}, fmt.Errorf("invalid JSONL line: %w", err)
	}
	if len(raw) < 5 {
		return Op{}, fmt.Errorf("op array must have at least 5 elements, got %d", len(raw))
	}

	var op Op

	if err := json.Unmarshal(raw[0], &op.Type); err != nil {
		return Op{}, fmt.Errorf("invalid op type: %w", err)
	}
	if !ValidOpTypes[op.Type] {
		return Op{}, fmt.Errorf("unknown op type: %s", op.Type)
	}

	if err := json.Unmarshal(raw[1], &op.TargetID); err != nil {
		return Op{}, fmt.Errorf("invalid target_id: %w", err)
	}
	if err := json.Unmarshal(raw[2], &op.Timestamp); err != nil {
		return Op{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	if err := json.Unmarshal(raw[3], &op.WorkerID); err != nil {
		return Op{}, fmt.Errorf("invalid worker_id: %w", err)
	}
	if err := json.Unmarshal(raw[4], &op.Payload); err != nil {
		return Op{}, fmt.Errorf("invalid payload: %w", err)
	}

	return op, nil
}

// MarshalOp serializes an Op to positional array JSONL format.
func MarshalOp(op Op) ([]byte, error) {
	payload, err := json.Marshal(op.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	arr := []interface{}{op.Type, op.TargetID, op.Timestamp, op.WorkerID, json.RawMessage(payload)}
	return json.Marshal(arr)
}
