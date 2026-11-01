package doctor

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/redact"
)

// defaultLocalOnlyOpsStaleAfter is the G2.2 / D11 default staleness window for
// unpublished local ops commits on _armature when config omits
// local_only_ops_stale_hours.
const defaultLocalOnlyOpsStaleAfter = 24 * time.Hour

const (
	d11OpsBranch       = "_armature"
	d11RunNS           = "refs/remotes/origin/_armature-d11-run"
	d11RemediationHint = "arm push-ops"
)

// d11Git is the adapter surface D11 probes need. *adapters.Client satisfies it.
// Kept local (not shared with opsbackup) so S12-T1G can keep moving without a
// shared-package churn: D11 needs ahead/age classification beyond tip evidence.
type d11Git interface {
	FetchBranchToRef(branch, dstRef string) error
	ResolveRevision(rev string) (string, error)
	RevListCount(rangeSpec string) (int, error)
	FirstParentAfter(base, head string) ([]string, error)
	CommitterUnix(sha string) (int64, error)
	DeleteRef(ref string) error
}

type d11Outcome string

const (
	d11Healthy        d11Outcome = "healthy"
	d11Missing        d11Outcome = "missing"
	d11Stale          d11Outcome = "stale"
	d11CouldNotVerify d11Outcome = "could-not-verify"
)

type d11Observation struct {
	outcome        d11Outcome
	localOnlyCount int
	localOnlyAge   time.Duration
	detail         string
}

func checkD11OpsRemote(worktreePath, configPath string, now time.Time) Finding {
	skip := Finding{Check: "D11", Severity: SeverityOK, Message: "Ops remote tracking not checked"}
	if strings.TrimSpace(worktreePath) == "" {
		return skip
	}
	info, err := os.Stat(worktreePath)
	if err != nil || !info.IsDir() {
		return skip
	}

	gc := adapters.New(worktreePath)
	stale := loadLocalOnlyOpsStaleAfter(configPath)
	if stale.convertErr != "" {
		detail := stale.convertErr
		return Finding{
			Check:    "D11",
			Severity: SeverityError,
			Message:  "Invalid local_only_ops_stale_hours: " + detail,
			Items:    []string{detail},
		}
	}
	obs := observeD11OpsRemote(gc, now, stale.after)
	return evaluateD11OpsRemote(obs, stale.fallbackWarn)
}

type d11StaleLoad struct {
	after        time.Duration
	fallbackWarn string // non-empty when unreadable config forced the 24h default
	convertErr   string // non-empty when configured hours fail duration conversion
}

func loadLocalOnlyOpsStaleAfter(configPath string) d11StaleLoad {
	def := d11StaleLoad{after: defaultLocalOnlyOpsStaleAfter}
	if strings.TrimSpace(configPath) == "" {
		return def
	}
	// Same read + ValidatePresentFields path as D10 (checkD10ConfigHealth).
	data, err := readRepoConfigFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errNotRepoConfigJSON) {
			return def
		}
		detail := redact.Secrets(err.Error())
		def.fallbackWarn = "could not load local_only_ops_stale_hours; using default 24h: " + detail
		return def
	}
	// Present 0 / negative / overflow must fail here too — never omit→24h.
	for _, p := range config.ValidatePresentFields(data) {
		if strings.Contains(p, "local_only_ops_stale_hours") {
			return d11StaleLoad{convertErr: redact.Secrets(p)}
		}
	}
	cfg, err := config.StrictDecode(data)
	if err != nil {
		detail := redact.Secrets(err.Error())
		def.fallbackWarn = "could not load local_only_ops_stale_hours; using default 24h: " + detail
		return def
	}
	after, err := cfg.LocalOnlyOpsStaleAfter()
	if err != nil {
		return d11StaleLoad{convertErr: redact.Secrets(err.Error())}
	}
	return d11StaleLoad{after: after}
}

// observeD11OpsRemote classifies origin/_armature from fresh per-run evidence
// only. Positive remote-ref-absent → missing. Fresh tip + age/threshold →
// healthy or stale. Every other fetch/resolve/rev-list/age failure →
// could-not-verify. Never refreshes shared refs/remotes/origin/_armature
// (per-run FetchBranchToRef with empty --refmap=, same class as ops-backup).
// staleAfter must be a positive, overflow-checked duration from
// loadLocalOnlyOpsStaleAfter / Config.LocalOnlyOpsStaleAfter — never clamped
// back to 24h here.
func observeD11OpsRemote(g d11Git, now time.Time, staleAfter time.Duration) d11Observation {
	runRef, err := uniqueD11RunRef()
	if err != nil {
		return d11Observation{outcome: d11CouldNotVerify, detail: err.Error()}
	}
	defer func() {
		_ = g.DeleteRef(runRef) //nolint:errcheck // best-effort per-run cleanup
	}()

	fetchErr := g.FetchBranchToRef(d11OpsBranch, runRef)
	if fetchErr != nil {
		if adapters.IsRemoteRefMissing(fetchErr) {
			return d11Observation{outcome: d11Missing}
		}
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  redact.Secrets(fetchErr.Error()),
		}
	}
	tip, err := g.ResolveRevision(runRef)
	if err != nil {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  redact.Secrets(err.Error()),
		}
	}
	if tip == "" {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  "resolved empty tip for origin/_armature",
		}
	}

	ahead, err := g.RevListCount(runRef + "..HEAD")
	if err != nil {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  redact.Secrets(err.Error()),
		}
	}
	if ahead == 0 {
		return d11Observation{outcome: d11Healthy}
	}

	localOnly, err := g.FirstParentAfter(runRef, "HEAD")
	if err != nil {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  redact.Secrets(err.Error()),
		}
	}
	if len(localOnly) == 0 {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  fmt.Sprintf("rev-list reported %d local-only commit(s) but first-parent walk was empty", ahead),
		}
	}
	oldestUnix, err := g.CommitterUnix(localOnly[0])
	if err != nil {
		return d11Observation{
			outcome: d11CouldNotVerify,
			detail:  redact.Secrets(err.Error()),
		}
	}
	age := now.Sub(time.Unix(oldestUnix, 0))
	if age < 0 {
		age = 0
	}
	obs := d11Observation{
		localOnlyCount: ahead,
		localOnlyAge:   age,
	}
	if age >= staleAfter {
		obs.outcome = d11Stale
		return obs
	}
	obs.outcome = d11Healthy
	return obs
}

func evaluateD11OpsRemote(obs d11Observation, configFallbackWarn string) Finding {
	f := Finding{
		Check:    "D11",
		Severity: SeverityOK,
		Message:  "Ops remote tracking tip is verified",
	}
	switch obs.outcome {
	case d11Missing:
		f.Severity = SeverityWarning
		f.Message = "Ops branch has no verified remote tracking tip (origin/_armature missing)"
		f.Items = []string{d11RemediationHint}
	case d11Stale:
		f.Severity = SeverityWarning
		f.Message = fmt.Sprintf(
			"Local-only ops on _armature are stale (%d unpublished commit(s) for %s)",
			obs.localOnlyCount,
			obs.localOnlyAge.Round(time.Second),
		)
		f.Items = []string{d11RemediationHint}
	case d11CouldNotVerify:
		f.Severity = SeverityError
		detail := strings.TrimSpace(obs.detail)
		if detail == "" {
			detail = "unknown probe failure"
		}
		f.Message = "Could not verify ops remote tracking tip: " + detail
		f.Items = []string{detail}
	default:
		// healthy / empty → OK
	}
	warn := strings.TrimSpace(configFallbackWarn)
	if warn == "" {
		return f
	}
	// Fail-closed visibility: never silently apply the 24h default when config
	// existed but could not be loaded. D10 also errors on the same file.
	if f.Severity == SeverityOK {
		f.Severity = SeverityWarning
		f.Message = warn
	} else {
		f.Items = append([]string{warn}, f.Items...)
	}
	return f
}

// d11DoctorExit mirrors arm doctor exit semantics for a single D11 finding:
// errors → 1; warnings → 0 unless --strict.
func d11DoctorExit(f Finding, strict bool) int {
	switch f.Severity {
	case SeverityError:
		return 1
	case SeverityWarning:
		if strict {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func uniqueD11RunRef() (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("d11 run ref: %w", err)
	}
	return d11RunNS + "/" + hex.EncodeToString(nonce[:]), nil
}
