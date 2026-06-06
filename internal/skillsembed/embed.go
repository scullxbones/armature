// Package skillsembed exposes the embedded skills filesystem.
package skillsembed

import (
	"embed"
	"io/fs"
)

//go:embed skills plugin.json
var skillsFS embed.FS

// SkillsFS is the embedded filesystem containing all bundled skills and plugin configuration.
var SkillsFS fs.FS = skillsFS
