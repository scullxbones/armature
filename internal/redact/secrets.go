// Package redact strips credential-bearing substrings from text shown to operators.
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

var schemeURL = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s'"]+`)

var scpRemote = regexp.MustCompile(`(^|[\s'"])([^\s'":/@]+@)([^\s'":/]+):([^\s'"]+)`)

var tokenPrefix = regexp.MustCompile(`\b((?:gh[pousr]_|github_pat_|glpat-|xox[baprs]-))[A-Za-z0-9_-]+`)

var jwtToken = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)

func Secrets(s string) string {
	s = schemeURL.ReplaceAllStringFunc(s, redactSchemeURL)
	s = scpRemote.ReplaceAllString(s, `${1}${3}:***`)
	s = tokenPrefix.ReplaceAllString(s, `${1}***`)
	s = jwtToken.ReplaceAllString(s, `***`)
	return s
}

func redactSchemeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return raw
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Host == "" {
		return scheme + "://***"
	}
	out := scheme + "://" + u.Host
	if u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return out + "/***"
	}
	return out
}
