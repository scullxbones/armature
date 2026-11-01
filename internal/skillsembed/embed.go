// Package skillsembed exposes the embedded skills filesystem.
package skillsembed

import (
	"embed"
	"io/fs"
)

//go:embed skills plugin.json
var skillsFS embed.FS

// SkillsFS returns the embedded filesystem containing all bundled skills and plugin configuration.
func SkillsFS() fs.FS {
	return skillsFS
}
