package importbf_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/importbf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleCSV = `id,title,type,parent,scope
TSK-X1,First import,task,,internal/foo/*.go
TSK-X2,Second import,task,,"internal/bar/*.go,internal/baz/*.go"
`

func TestParseCSV(t *testing.T) {
	items, err := importbf.ParseCSV([]byte(sampleCSV))
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "TSK-X1", items[0].ID)
	assert.Equal(t, "task", items[0].Type)
	assert.Equal(t, []string{"internal/foo/*.go"}, items[0].Scope)
}

var sampleJSON = `[
  {"id":"TSK-J1","title":"JSON import","type":"story","scope":["internal/ops/**"]},
  {"id":"TSK-J2","title":"Another","type":"task","parent":"TSK-J1"}
]`

func TestParseJSON(t *testing.T) {
	items, err := importbf.ParseJSON([]byte(sampleJSON))
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "TSK-J1", items[0].ID)
	assert.Equal(t, "story", items[0].Type)
}

func TestImportedItemsHaveCorrectProvenance(t *testing.T) {
	items, _ := importbf.ParseCSV([]byte(sampleCSV))
	for _, item := range items {
		assert.Equal(t, "imported", item.Provenance.Method)
		assert.Equal(t, "inferred", item.Provenance.Confidence)
		assert.True(t, item.RequiresConfirmation)
	}
}
