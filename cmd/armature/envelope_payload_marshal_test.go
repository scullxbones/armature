package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/require"
)

func TestNewEnvelopeCmdPayloadTypesHaveNoMarshalJSON_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	samples := []any{
		worktreeRow{},
		validateFindingRow{},
		contextHistoryRow{},
		readyExplainRow{},
		readyIssueRow{},
		reviewAssessmentRow{},
		reviewBundleWriteRow{},
		versionRow{},
		WorkerStatus{},
		applyIssueRow{},
		doctorCheckRow{},
		output.IssueJSON{},
	}
	for _, sample := range samples {
		require.False(t, implementsJSONMarshaler(sample),
			"%T must not define MarshalJSON; nil/empty encoding stays Envelope's [] plus help", sample)
	}
}

func implementsJSONMarshaler(sample any) bool {
	if _, ok := sample.(json.Marshaler); ok {
		return true
	}
	rv := reflect.ValueOf(sample)
	if rv.Kind() == reflect.Pointer {
		return false
	}
	p := reflect.New(rv.Type())
	p.Elem().Set(rv)
	_, ok := p.Interface().(json.Marshaler)
	return ok
}
