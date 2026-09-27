// Package redact strips credential-bearing substrings from text shown to operators.
package redact

import "regexp"

var urlUserinfo = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)

var urlQueryValues = regexp.MustCompile(`([?&][^=?#\s]+=)([^&#\s'"]*)`)

var tokenPrefix = regexp.MustCompile(`\b((?:gh[pousr]_|github_pat_|glpat-|xox[baprs]-))[A-Za-z0-9_-]+`)

var jwtToken = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)

func Secrets(s string) string {
	s = urlUserinfo.ReplaceAllString(s, `${1}***@`)
	s = urlQueryValues.ReplaceAllString(s, `${1}***`)
	s = tokenPrefix.ReplaceAllString(s, `${1}***`)
	s = jwtToken.ReplaceAllString(s, `***`)
	return s
}
