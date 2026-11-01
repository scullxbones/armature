package review

import (
	_ "embed"
)

// ConformanceAssessmentSchemaJSON is the ConformanceAssessment JSON Schema
// shipped with the binary so user repos without docs/schemas can still author
// assessments (`arm review schema`).
//
//go:embed schema/conformance-assessment.schema.json
var ConformanceAssessmentSchemaJSON []byte
