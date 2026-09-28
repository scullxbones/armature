// Package issueref holds the citation-coverage projection shared by
// materialize and traceability so neither package imports the other.
package issueref

type IssueRef struct {
	ID                      string
	SourceLinkCount         int
	CitationAcceptanceCount int
	Confidence              string
}
