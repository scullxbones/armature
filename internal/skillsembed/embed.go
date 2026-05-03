// Package skillsembed exposes the embedded skills filesystem.
package skillsembed

import (
	"embed"
	"io/fs"
)

//go:embed skills
var skillsFS embed.FS

// SkillsFS is the embedded filesystem containing all bundled skills.
var SkillsFS fs.FS = skillsFS
