// Package redact strips credential-bearing substrings from text shown to operators.
package redact

import "regexp"

var urlUserinfo = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)

var githubTokenPrefix = regexp.MustCompile(`\b(gh[pousr]_|github_pat_)[A-Za-z0-9_]+`)

func Secrets(s string) string {
	s = urlUserinfo.ReplaceAllString(s, `${1}***@`)
	s = githubTokenPrefix.ReplaceAllString(s, `${1}***`)
	return s
}
