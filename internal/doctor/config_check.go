package doctor

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/config"
)

const repoConfigFileName = "config.json"

var errNotRepoConfigJSON = errors.New("path is not the repo's config.json")

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

	data, err := readRepoConfigFile(configPath)
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

func readRepoConfigFile(configPath string) ([]byte, error) {
	cleaned := filepath.Clean(configPath)
	if filepath.Base(cleaned) != repoConfigFileName {
		return nil, errNotRepoConfigJSON
	}
	return os.ReadFile(cleaned)
}
