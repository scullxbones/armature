package main

import (
	"reflect"
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/require"
)

func TestNewEnvelopeCmdPayloadTypesHaveNoMarshalJSON_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	samples := []any{
		[]worktreeRow{},
		[]validateFindingRow{},
		[]contextHistoryRow{},
		[]readyExplainRow{},
		[]readyIssueRow{},
		[]reviewAssessmentRow{},
		[]reviewBundleWriteRow{},
		[]versionRow{},
		[]WorkerStatus{},
		[]applyIssueRow{},
		[]doctorCheckRow{},
	}
	for _, sample := range samples {
		elem := reflect.TypeOf(sample).Elem()
		_, ok := reflect.PtrTo(elem).MethodByName("MarshalJSON")
		require.False(t, ok, "%s must not define MarshalJSON; nil/empty encoding stays Envelope's [] + help", elem)
	}

	_, ok := reflect.PtrTo(reflect.TypeOf(output.IssueJSON{})).MethodByName("MarshalJSON")
	require.False(t, ok)
}
