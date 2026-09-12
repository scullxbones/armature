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

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/cobra"
)

// Mode is one structured invocation of a cobra command: the command path plus
// any Artifact Output selecting flag. --field is a projection (N1.5/N9.6) and
// is never a mode.
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

// EnumerateModes walks the cobra tree and returns every structured command mode.
// Grouping-only commands (no Run/RunE) are skipped. Cobra's help command is
// skipped. Hidden leaves are included so Protocol Output cannot hide by Hidden.
func EnumerateModes(root *cobra.Command) []Mode {
	if root == nil {
		return nil
	}
	var modes []Mode
	walkCommands(root, func(cmd *cobra.Command) {
		if !enumerableCommand(root, cmd) {
			return
		}
		modes = append(modes, modesForCommand(root, cmd)...)
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
func Lint(root *cobra.Command, goldenDir string) error {
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

func enumerableCommand(root, cmd *cobra.Command) bool {
	if cmd == nil || cmd.Name() == "help" {
		return false
	}
	hasRun := cmd.Run != nil || cmd.RunE != nil
	if !hasRun {
		return false
	}
	if cmd == root {
		return true
	}
	// Visible subcommands mean this RunE is group help, not a result mode.
	return !cmd.HasAvailableSubCommands()
}

func modesForCommand(root, cmd *cobra.Command) []Mode {
	path := relativeCommandPath(root, cmd)
	ann := cmd.Annotations
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

func relativeCommandPath(root, cmd *cobra.Command) string {
	full := strings.TrimSpace(cmd.CommandPath())
	rootName := strings.TrimSpace(root.Name())
	if full == rootName {
		return ""
	}
	prefix := rootName + " "
	if strings.HasPrefix(full, prefix) {
		return strings.TrimPrefix(full, prefix)
	}
	return full
}

func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		walkCommands(sub, visit)
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
		return nil, "", fmt.Errorf("%s: missing golden fixture %s.json (agent-facing modes require a conforming envelope; Artifact Output modes require the cited foreign shape)", m.label(), m.ID())
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
		return fmt.Errorf("Artifact Output must not use the Agent Output Contract envelope; cite %s", m.Citation)
	}
	if m.Citation == CitationShellCompletionGrammar {
		return nil
	}
	if strings.HasSuffix(m.Citation, ".json") {
		if m.Selector == "schema" {
			return checkJSONSchemaDocument(trimmed)
		}
		return validateAgainstCitedSchema(m.Citation, trimmed)
	}
	return fmt.Errorf("unsupported artifact citation %q", m.Citation)
}

func checkJSONSchemaDocument(body []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("schema artifact must be a JSON Schema document: %w", err)
	}
	if _, ok := obj["$schema"]; !ok {
		return fmt.Errorf("schema artifact must include $schema")
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("fixture.schema.json", bytes.NewReader(body)); err != nil {
		return fmt.Errorf("schema artifact is not a JSON Schema: %w", err)
	}
	if _, err := compiler.Compile("fixture.schema.json"); err != nil {
		return fmt.Errorf("schema artifact does not compile: %w", err)
	}
	return nil
}

func validateAgainstCitedSchema(citation string, body []byte) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	schemaPath := citation
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(root, citation)
	}
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read cited schema %s: %w", citation, err)
	}
	compiler := jsonschema.NewCompiler()
	url := "file://" + filepath.ToSlash(schemaPath)
	if err := compiler.AddResource(url, bytes.NewReader(schemaBytes)); err != nil {
		return fmt.Errorf("load cited schema %s: %w", citation, err)
	}
	sch, err := compiler.Compile(url)
	if err != nil {
		return fmt.Errorf("compile cited schema %s: %w", citation, err)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("artifact fixture must be JSON: %w", err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("artifact does not match %s: %w", citation, err)
	}
	return nil
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate conformance.go")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "docs", "schemas")); err != nil {
		return "", fmt.Errorf("repo root from %s: %w", file, err)
	}
	return root, nil
}
