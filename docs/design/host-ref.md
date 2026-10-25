# Host ref

An Armature issue can name GitHub issue URLs. The names are ordered. One of them is current. A separate command can print that host item's title and body once. The ticket stays the SOC 2 approval record. The URL is only the join between that ticket and the coding record.

This file is the spec for that behavior. It does not add Linear or Jira. A Linear URL is a parse error until a later change adds a finished Linear parse-and-fetch pair. That pair is out of scope here.

## Commands

`arm ref` is one flat verb. It is not a command group.

```text
arm ref ISSUE URL [--role ROLE]
arm ref ISSUE URL --withdraw
arm ref-read ISSUE
arm ref-read ISSUE --url URL
arm list --external-ref URL
```

`arm ref` takes one issue id and one URL. `--role` is a label such as `support` or `engineering`. The log does not close that list. An empty role is allowed. `--role` and `--withdraw` together are a command error. One invocation does not both record a live name and withdraw.

`arm ref-read` prints the title, a blank line, and the body to stdout. With no `--url` it reads the current live name. With `--url` it reads that live name and rejects a URL that is not live. It appends no op. It writes no file. It does not update the host. It does not copy host status onto the issue.

`--format json` on `arm ref-read` prints a JSON object with `url`, `title`, and `body`. It still stores nothing.

`arm show`, `arm list`, and `render-context` print the ordered names, the role, and which name is current. They print no body. They open no connection. `arm list --external-ref` parses the flag value with `ParseGitHubIssueURL` and keeps issues whose live names contain that URL.

`arm ref-read` is a hyphenated root verb. `TestNoHyphenatedRootTopLevel` rejects a new hyphenated root command unless the name is in the signal allowlist in `cmd/armature/grammar_test.go`. Add `ref-read` to that allowlist in the same change that registers the command. Do not invent a group to avoid the hyphen. ADR 0011 is why the allowlist exists. The settled name stays `ref-read`.

## Op and replay

The op type is `external-ref`. Classify it false in `classifiedValidity`, beside `assign`. An unclassified type is validity-affecting. Payload fields are `external_ref`, `external_role`, and `withdraw_external_ref`. There is no approval field and no body field.

```go
type HostRef struct {
	URL       string `json:"url"`
	Role      string `json:"role,omitempty"`
	At        int64  `json:"at"`
	WorkerID  string `json:"worker_id"`
	Withdrawn bool   `json:"withdrawn,omitempty"`
}
```

`Issue.HostRefs` is that slice in replay order.

Replay always appends one `HostRef`. It does not call `slices.Contains`. It does not decide the no-op. The command does that before it writes.

Current is the latest live name. A name is live when a later event has not withdrawn it. Recording the same canonical URL and role again, while that pair is already the latest live event, appends nothing. Recording it again after a different name appends a new event and makes it current. A withdraw of a URL that is not live appends nothing. A withdraw of a live URL appends a withdrawn event. Re-adding that URL later appends a live event and makes it current.

```go
func CurrentHostRef(refs []HostRef) (HostRef, bool) {
	live := map[string]HostRef{}
	order := []string{}
	for _, ref := range refs {
		if ref.Withdrawn {
			delete(live, ref.URL)
			order = dropURL(order, ref.URL)
			continue
		}
		order = dropURL(order, ref.URL)
		order = append(order, ref.URL)
		live[ref.URL] = ref
	}
	if len(order) == 0 {
		return HostRef{}, false
	}
	return live[order[len(order)-1]], true
}
```

`dropURL` returns the order with that URL removed. An empty result means there is no live name.

The index stores the current URL and every live URL, so list can filter without opening each issue. The full order, including withdrawn events and roles, stays on the issue.

The command skips the append only when the materialized history already shows the no-op. That skip is the [ADR 0018](../adr/0018-payload-keyed-op-idempotency.md) rule. A repeated no-op still has to publish a local op the origin does not have yet. `TestTransitionIdenticalRetryPublishesUnpublishedLocalOp_REQ_OPS_PUBLISH` in `cmd/armature/ops_publish_test.go` pins that publish rule for `arm transition`.

Do not copy `applySourceLink`.

## Parse and fetch

`ParseGitHubIssueURL` returns a `GitHubIssue`. `FetchGitHubIssue` accepts only that value. A raw string is not fetchable. There is no `Host` interface and no adapter registry. Neither function is published with an unimplemented body.

```go
type PlanText struct {
	URL   string
	Title string
	Body  string
}

type GitHubIssue struct {
	canonical string
}

func (g GitHubIssue) URL() string { return g.canonical }

func ParseGitHubIssueURL(raw string) (GitHubIssue, error)

func FetchGitHubIssue(ctx context.Context, issue GitHubIssue) (PlanText, error)
```

Only `ParseGitHubIssueURL` constructs a `GitHubIssue`.

GitHub rules, for `https://github.com/{owner}/{repo}/issues/{n}` only.

- Lowercase owner and repo before storing.
- Store `n` as a decimal integer with no leading zeros.
- Drop a query and a fragment.
- Reject a trailing slash and any extra path segment.
- Reject a path that contains `/pull/`. That reject is a spelling check. An issue and a pull request can share a number. This parser does not call the API to tell them apart.
- `github.com` only. A GitHub Enterprise host is a parse error.

A Linear URL, any other unknown host, and a malformed GitHub URL return the same parse error.

`FetchGitHubIssue` calls `GET https://api.github.com/repos/{owner}/{repo}/issues/{n}` and reads `title` and `body`. The token comes from `GITHUB_TOKEN` in the environment. It is not a flag and not a file in the repo. A missing token is an error and writes nothing. The call does not update the host.

`FetchGitHubIssue` is not called from `render-context`, `ready`, `show`, or `list`.

## What this does not do

The host ticket is the system of record for proof that someone approved the change. CC8.1 asks for a process to approve system changes prior to implementation. That proof stays on the ticket. The ops log can show the names, the gate evidence, `done`, and `merged`. None of those is the approval.

This work does not add a ticket body to git, a source link to a ticket, a status sync, a write to the host, a `Fixes` trailer, or an approval field. `done` stays a worker report. `merged` stays the landing check. A claim stays a reservation.
