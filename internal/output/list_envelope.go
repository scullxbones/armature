package output

import (
	"io"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

const ListShowHelp = "arm show <id> for outcome, scope, and acceptance"

// listStatusOrder is display priority for --group output — lower number appears first.
var listStatusOrder = map[string]int{
	ops.StatusInProgress: 0,
	ops.StatusClaimed:    1,
	ops.StatusDone:       2,
	ops.StatusOpen:       3,
	ops.StatusBlocked:    4,
	ops.StatusMerged:     5,
	ops.StatusCancelled:  6,
}

// ListIssue is the N4 default list row: id, type, status, title only.
type ListIssue struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// ListGroup is the structured --group adjunct: status buckets in workflow order.
// Issue rows stay in the issues payload (N2); groups only name ids.
type ListGroup struct {
	Status string   `json:"status"`
	IDs    []string `json:"ids"`
}

// ListStatusRank returns the --group display order for a status.
func ListStatusRank(status string) int {
	if n, ok := listStatusOrder[status]; ok {
		return n
	}
	return 99
}

// ListRows builds N4 list rows for ids in the given order.
func ListRows(index materialize.Index, ids []string) []ListIssue {
	rows := make([]ListIssue, 0, len(ids))
	for _, id := range ids {
		e := index[id]
		rows = append(rows, ListIssue{
			ID:     id,
			Type:   e.Type,
			Status: e.Status,
			Title:  e.Title,
		})
	}
	return rows
}

// ListGroupsByStatus buckets ids by status in workflow order.
func ListGroupsByStatus(index materialize.Index, ids []string) []ListGroup {
	buckets := make(map[string][]string)
	for _, id := range ids {
		s := index[id].Status
		buckets[s] = append(buckets[s], id)
	}
	statuses := make([]string, 0, len(buckets))
	for s := range buckets {
		statuses = append(statuses, s)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return ListStatusRank(statuses[i]) < ListStatusRank(statuses[j])
	})
	groups := make([]ListGroup, 0, len(statuses))
	for _, status := range statuses {
		members := buckets[status]
		sort.Strings(members)
		groups = append(groups, ListGroup{Status: status, IDs: members})
	}
	return groups
}

// ListHelp is the trailing help for a list envelope.
func ListHelp(filtered bool, n int) []string {
	if n == 0 {
		reason := "no issues in the repository"
		if filtered {
			reason = "no issues match the filter"
		}
		return []string{reason, ListShowHelp}
	}
	return []string{ListShowHelp}
}

// WriteListEnvelope emits the compact agent list object {count,issues,help}
// and optional groups adjunct. This is the live arm list json/agent path.
func WriteListEnvelope(w io.Writer, rows []ListIssue, groups []ListGroup, grouped, filtered bool) error {
	env, err := NewEnvelope("issues", rows, ListHelp(filtered, len(rows)))
	if err != nil {
		return err
	}
	if grouped {
		if err := env.AddAdjunct("groups", groups); err != nil {
			return err
		}
	}
	return WriteEnvelope(w, env)
}
