package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Command is one cobra command as the shape lint sees it. cmd/armature maps the
// live cobra tree onto this type so internal/output never imports cobra.
type Command struct {
	Name             string
	Annotations      map[string]string
	HasRun           bool
	HasAvailableSubs bool
	Children         []*Command
}

// Mode is one structured invocation: the command path plus any Artifact Output
// selecting flag. --field is a projection (N1.5/N9.6) and is never a mode.
type Mode struct {
	Path     string
	Selector string
	Channel  Channel
	Citation string
}

// ID is the stable fixture stem for a mode (spaces become dots).
func (m Mode) ID() string {
	path := m.Path
	if path == "" {
		path = "_root"
	}
	name := strings.ReplaceAll(path, " ", ".")
	if m.Selector != "" {
		name += "." + m.Selector
	}
	switch m.Channel {
	case ChannelArtifactOutput:
		return name + ".artifact"
	case ChannelProtocolOutput:
		return name + ".protocol"
	default:
		return name
	}
}

func (m Mode) label() string {
	label := m.Path
	if label == "" {
		label = "arm"
	}
	if m.Selector != "" {
		label += " --" + m.Selector
	}
	return label
}

// DefaultGoldenDir is internal/output/testdata/golden next to this package.
func DefaultGoldenDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "golden")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "golden")
}

// EnumerateModes walks a command tree and returns every structured mode.
// Grouping-only commands (no run function) are skipped. The help command is
// skipped. Hidden leaves remain so Protocol Output cannot hide by Hidden.
func EnumerateModes(root *Command) []Mode {
	if root == nil {
		return nil
	}
	var modes []Mode
	walkCommands(root, "", true, func(cmd *Command, path string, isRoot bool) {
		if !enumerableCommand(cmd, isRoot) {
			return
		}
		modes = append(modes, modesForCommand(path, cmd.Annotations)...)
	})
	sort.Slice(modes, func(i, j int) bool {
		if modes[i].Path != modes[j].Path {
			return modes[i].Path < modes[j].Path
		}
		if modes[i].Selector != modes[j].Selector {
			return modes[i].Selector < modes[j].Selector
		}
		return modes[i].Channel < modes[j].Channel
	})
	return modes
}

// Lint enumerates modes from root and checks each agent-facing mode against a
// conforming golden, and each Artifact Output mode against its cited foreign
// shape. Protocol Output modes are exempt only when Classify says so.
func Lint(root *Command, goldenDir string) error {
	return lintModes(EnumerateModes(root), goldenDir)
}

func lintModes(modes []Mode, goldenDir string) error {
	var errs []string
	for _, m := range modes {
		if m.Channel == ChannelProtocolOutput {
			continue
		}
		body, path, err := readFixture(goldenDir, m)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if m.Channel == ChannelArtifactOutput {
			if err := checkArtifactFixture(m, body); err != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): %v", m.label(), path, err))
			}
			continue
		}
		if err := checkEnvelopeFixture(m, body); err != nil {
			errs = append(errs, fmt.Sprintf("%s (%s): %v", m.label(), path, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("envelope shape lint failed:\n  %s", strings.Join(errs, "\n  "))
}

func enumerableCommand(cmd *Command, isRoot bool) bool {
	if cmd == nil || cmd.Name == "help" {
		return false
	}
	if !cmd.HasRun {
		return false
	}
	if isRoot {
		return true
	}
	// Visible subcommands mean this run function is group help, not a result mode.
	return !cmd.HasAvailableSubs
}

func modesForCommand(path string, ann map[string]string) []Mode {
	if Classify(ann) == ChannelProtocolOutput {
		return []Mode{{Path: path, Channel: ChannelProtocolOutput}}
	}
	citation := ""
	if ann != nil {
		citation = strings.TrimSpace(ann[ArtifactCitationKey])
	}
	marked := Channel(ann[ChannelAnnotationKey]) == ChannelArtifactOutput && citation != ""
	if !marked {
		return []Mode{{Path: path, Channel: ChannelAgentFacing}}
	}
	anySet := splitCSV(ann[ArtifactWhenAnyFlagSetKey])
	allUnset := splitCSV(ann[ArtifactWhenAllFlagsUnsetKey])
	if len(anySet) == 0 && len(allUnset) == 0 {
		return []Mode{{Path: path, Channel: ChannelArtifactOutput, Citation: citation}}
	}
	var modes []Mode
	for _, flag := range anySet {
		modes = append(modes, Mode{
			Path: path, Selector: flag, Channel: ChannelArtifactOutput, Citation: citation,
		})
	}
	if len(anySet) > 0 {
		modes = append(modes, Mode{Path: path, Channel: ChannelAgentFacing})
	}
	if len(allUnset) > 0 {
		modes = append(modes, Mode{
			Path: path, Channel: ChannelArtifactOutput, Citation: citation,
		})
		for _, flag := range allUnset {
			modes = append(modes, Mode{
				Path: path, Selector: flag, Channel: ChannelAgentFacing,
			})
		}
	}
	return modes
}

func walkCommands(cmd *Command, parentPath string, isRoot bool, visit func(*Command, string, bool)) {
	path := parentPath
	if !isRoot {
		if path == "" {
			path = cmd.Name
		} else {
			path += " " + cmd.Name
		}
	}
	visit(cmd, path, isRoot)
	for _, sub := range cmd.Children {
		walkCommands(sub, path, false, visit)
	}
}

func readFixture(goldenDir string, m Mode) ([]byte, string, error) {
	base := filepath.Join(goldenDir, m.ID())
	candidates := []string{base + ".json", base + ".sh", base + ".txt"}
	var found []string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return nil, "", fmt.Errorf(
			"%s: missing golden fixture %s.json (agent-facing modes need an envelope; Artifact Output modes need the cited shape)",
			m.label(), m.ID())
	}
	if len(found) > 1 {
		return nil, "", fmt.Errorf("%s: multiple goldens for mode %s", m.label(), m.ID())
	}
	body, err := os.ReadFile(found[0])
	if err != nil {
		return nil, found[0], fmt.Errorf("%s: read %s: %w", m.label(), found[0], err)
	}
	return body, found[0], nil
}

func checkEnvelopeFixture(m Mode, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return fmt.Errorf("fixture is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("fixture must be one JSON object: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("fixture must contain exactly one JSON value")
	}
	if _, ok := raw["payload"]; ok {
		return fmt.Errorf("payload key must be a command-declared name, not the literal key %q", "payload")
	}
	countRaw, ok := raw["count"]
	if !ok {
		return fmt.Errorf("missing count")
	}
	count, err := parseCount(countRaw)
	if err != nil {
		return err
	}
	helpRaw, ok := raw["help"]
	if !ok {
		return fmt.Errorf("missing help")
	}
	help, err := parseHelp(helpRaw)
	if err != nil {
		return err
	}
	payloadKey, payloadLen, err := payloadArray(trimmed, raw)
	if err != nil {
		return err
	}
	if payloadLen != count {
		return fmt.Errorf("count %d does not equal payload %q length %d", count, payloadKey, payloadLen)
	}
	if count == 0 && len(help) == 0 {
		return fmt.Errorf("empty state must carry help naming why the result is empty")
	}
	if m.Path == "list" {
		if payloadKey != "issues" {
			return fmt.Errorf("list payload key must be issues, got %q", payloadKey)
		}
		if err := checkListRows(raw[payloadKey]); err != nil {
			return err
		}
		if !helpMentionsShow(help) {
			return fmt.Errorf("list help must point at arm show")
		}
	}
	if m.Path == "ready" || m.Path == "" {
		if payloadKey != "issues" {
			return fmt.Errorf("%s payload key must be issues, got %q", m.label(), payloadKey)
		}
	}
	if m.Path == "version" && payloadKey != "versions" {
		return fmt.Errorf("version payload key must be versions, got %q", payloadKey)
	}
	return nil
}

func parseCount(raw json.RawMessage) (int, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("count must be a JSON number")
	}
	if strings.ContainsAny(n.String(), ".eE") {
		return 0, fmt.Errorf("count must be an integer")
	}
	v, err := strconv.Atoi(n.String())
	if err != nil || v < 0 {
		return 0, fmt.Errorf("count must be an integer >= 0")
	}
	return v, nil
}

func parseHelp(raw json.RawMessage) ([]string, error) {
	if string(raw) == "null" {
		return nil, fmt.Errorf("help must be an array, not null")
	}
	var help []string
	if err := json.Unmarshal(raw, &help); err != nil {
		return nil, fmt.Errorf("help must be an array of strings")
	}
	if help == nil {
		return nil, fmt.Errorf("help must be an array, not null")
	}
	for i, h := range help {
		if h == "" {
			return nil, fmt.Errorf("help[%d] must be a non-empty string", i)
		}
	}
	return help, nil
}

func payloadArray(body []byte, raw map[string]json.RawMessage) (string, int, error) {
	keys, err := topLevelKeys(body)
	if err != nil {
		return "", 0, err
	}
	if len(keys) == 0 || keys[0] != "count" {
		return "", 0, fmt.Errorf("count must be the first envelope member")
	}
	if keys[len(keys)-1] != "help" {
		return "", 0, fmt.Errorf("help must trail the payload")
	}
	if len(keys) < 3 {
		return "", 0, fmt.Errorf("missing payload array")
	}
	key := keys[1]
	if key == "help" || key == "count" {
		return "", 0, fmt.Errorf("missing payload array")
	}
	if key == "payload" {
		return "", 0, fmt.Errorf("payload key must be a command-declared name, not the literal key %q", "payload")
	}
	val, ok := raw[key]
	if !ok {
		return "", 0, fmt.Errorf("missing payload array")
	}
	n, isArr, err := jsonArrayLen(val)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", key, err)
	}
	if !isArr {
		return "", 0, fmt.Errorf("payload %q must be a JSON array", key)
	}
	return key, n, nil
}

func topLevelKeys(raw []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("fixture must be a JSON object: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("fixture must be a JSON object")
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("object member name must be a string")
		}
		keys = append(keys, key)
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func jsonArrayLen(raw json.RawMessage) (int, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return 0, false, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return 0, false, fmt.Errorf("payload must be a JSON array")
	}
	if arr == nil {
		return 0, false, fmt.Errorf("payload must be an array, not null")
	}
	return len(arr), true, nil
}

func checkListRows(raw json.RawMessage) error {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return fmt.Errorf("list payload must be an array of objects")
	}
	for i, row := range rows {
		if len(row) != 4 {
			return fmt.Errorf("list row %d must have exactly id, type, status, title", i)
		}
		for _, key := range []string{"id", "type", "status", "title"} {
			if _, ok := row[key]; !ok {
				return fmt.Errorf("list row %d missing %s", i, key)
			}
		}
		if _, ok := row["outcome"]; ok {
			return fmt.Errorf("list row %d must not include outcome", i)
		}
	}
	return nil
}

func helpMentionsShow(help []string) bool {
	for _, h := range help {
		if strings.Contains(h, "arm show") {
			return true
		}
	}
	return false
}

func checkArtifactFixture(m Mode, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return fmt.Errorf("artifact fixture is empty")
	}
	if err := checkEnvelopeFixture(m, body); err == nil {
		return fmt.Errorf("artifact output must not use the Agent Output Contract envelope; cite %s", m.Citation)
	}
	switch m.Citation {
	case CitationShellCompletionGrammar:
		return nil
	case CitationReviewBundleSchema:
		return checkReviewBundleShape(trimmed)
	case CitationPlanSchema:
		if m.Selector == "schema" {
			return checkJSONSchemaDocument(trimmed)
		}
		return checkPlanInstanceShape(trimmed)
	default:
		return fmt.Errorf("unsupported artifact citation %q", m.Citation)
	}
}

func checkJSONSchemaDocument(body []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("schema artifact must be a JSON Schema document: %w", err)
	}
	for _, key := range []string{"$schema", "type", "properties"} {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("schema artifact must include %s", key)
		}
	}
	return nil
}

func checkReviewBundleShape(body []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("review bundle must be JSON: %w", err)
	}
	for _, key := range []string{"schema_version", "bundle_id", "issue", "contract", "delivery", "fingerprints"} {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("review bundle missing %s (cited by %s)", key, CitationReviewBundleSchema)
		}
	}
	return nil
}

func checkPlanInstanceShape(body []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("plan instance must be JSON: %w", err)
	}
	for _, key := range []string{"version", "title", "issues"} {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("plan instance missing %s (cited by %s)", key, CitationPlanSchema)
		}
	}
	return nil
}
