package harnesspolicy

import "encoding/json"

func PolicyFromFacts(facts IssueFacts, knownSourceIDs map[string]struct{}) IssuePolicy {
	return IssuePolicy{
		ID:         facts.ID,
		Title:      facts.Title,
		Scope:      append([]string(nil), facts.Scope...),
		Acceptance: append(json.RawMessage(nil), facts.Acceptance...),
		Citations:  resolveCitationChecks(facts, knownSourceIDs),
	}
}

func resolveCitationChecks(facts IssueFacts, knownSourceIDs map[string]struct{}) []CitationCheck {
	if len(facts.SourceLinks) == 0 {
		return nil
	}

	emptySourceEntryIDAcceptsAll, accepted := citationAcceptanceIndex(facts.CitationAcceptances)

	checks := make([]CitationCheck, 0, len(facts.SourceLinks))
	for _, link := range facts.SourceLinks {
		if link.SourceEntryID == "" {
			continue
		}
		if _, ok := knownSourceIDs[link.SourceEntryID]; !ok {
			checks = append(checks, CitationCheck{
				SourceEntryID: link.SourceEntryID,
				Accepted:      emptySourceEntryIDAcceptsAll,
			})
			continue
		}
		checks = append(checks, CitationCheck{
			SourceEntryID: link.SourceEntryID,
			Accepted:      emptySourceEntryIDAcceptsAll || accepted[link.SourceEntryID],
		})
	}
	return checks
}

func citationAcceptanceIndex(acceptances []CitationAcceptanceFact) (emptySourceEntryIDAcceptsAll bool, byID map[string]bool) {
	byID = make(map[string]bool, len(acceptances))
	for _, acceptance := range acceptances {
		if acceptance.SourceEntryID == "" {
			emptySourceEntryIDAcceptsAll = true
			continue
		}
		byID[acceptance.SourceEntryID] = true
	}
	return emptySourceEntryIDAcceptsAll, byID
}
