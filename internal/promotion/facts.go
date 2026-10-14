package promotion

type AssessmentFact struct {
	BaseSHA string
	HeadSHA string
}

type IssueFacts struct {
	ID                     string
	Status                 string
	Children               []string
	Base                   string
	Tip                    string
	IntegrationBranch      string
	AssessmentAttestations []AssessmentFact
}
