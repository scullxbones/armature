package doctor

import (
	"errors"
	"os"

	"github.com/scullxbones/armature/internal/config"
)

// CheckD10ConfigHealth validates that config.json decodes strictly (unknown
// fields rejected by key) and that every present field is within its valid
// range. A missing file fails open, matching D1's I/O policy. The check ID is
// D10 because live D9 is unrecognized managed worktrees.
func CheckD10ConfigHealth(configPath string) Finding {
	f := Finding{
		Check:    "D10",
		Severity: SeverityOK,
		Message:  "Config decodes strictly and present fields are in range",
	}
	if configPath == "" {
		return f
	}

	data, err := os.ReadFile(configPath) //nolint:gosec // path is the repo's config.json
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return f
		}
		f.Severity = SeverityError
		f.Message = "Config.json could not be read"
		f.Items = []string{err.Error()}
		return f
	}

	problems := config.ValidatePresentFields(data)
	if len(problems) > 0 {
		f.Severity = SeverityError
		f.Message = "Config.json failed strict decode or range validation"
		f.Items = problems
	}
	return f
}
