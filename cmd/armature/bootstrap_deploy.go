package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// deploySkills copies all skills from src (rooted at the "skills" directory)
// into dest, creating subdirectories as needed. It is idempotent — existing
// files are overwritten.
func deploySkills(src fs.FS, dest string) error {
	const skillsRoot = "skills"

	return fs.WalkDir(src, skillsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute the path relative to the skills root.
		rel := strings.TrimPrefix(path, skillsRoot)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		rel = strings.TrimPrefix(rel, "/")

		target := filepath.Join(dest, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}

		return copyFile(src, path, target)
	})
}

// deployFlatSkills writes a flat <name>.md file alongside each skill directory so the
// Claude Code Skill tool can load skills by name. The Skill tool looks up skills as
// <name>.md or <name>/SKILL.md; when a directory is found first, the tool returns
// "Unknown skill". Writing a flat file makes both the slash-command (directory) and
// Skill tool (flat file) work simultaneously.
// Reference paths in the skill (e.g., "references/guide.md") are rewritten to
// "<skill-name>/references/guide.md" so they resolve correctly in the flat file location.
func deployFlatSkills(src fs.FS, dest string) error {
	const skillsRoot = "skills"

	entries, err := fs.ReadDir(src, skillsRoot)
	if err != nil {
		return fmt.Errorf("read skills root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillFile := skillsRoot + "/" + name + "/SKILL.md"
		target := filepath.Join(dest, name+".md")
		if err := copySkillWithRewrittenRefs(src, skillFile, name, target); err != nil {
			return fmt.Errorf("deploy flat skill %s: %w", name, err)
		}
	}
	return nil
}

// copySkillWithRewrittenRefs reads a skill's SKILL.md file from the embedded FS,
// rewrites all occurrences of "references/" to "<skill-name>/references/" to fix
// relative reference paths, and writes the result to destPath with mode 0644.
func copySkillWithRewrittenRefs(src fs.FS, srcPath, skillName, destPath string) error {
	content, err := fs.ReadFile(src, srcPath)
	if err != nil {
		return fmt.Errorf("read source %s: %w", srcPath, err)
	}

	rewritten := strings.ReplaceAll(string(content), "references/", skillName+"/references/")

	if err := os.WriteFile(destPath, []byte(rewritten), 0o600); err != nil {
		return fmt.Errorf("write dest %s: %w", destPath, err)
	}

	return nil
}

// deployPlugin copies the plugin.json file from src to dest, creating the
// destination directory as needed. It is idempotent — existing files are overwritten.
func deployPlugin(src fs.FS, dest string) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create plugin directory %s: %w", dest, err)
	}

	target := filepath.Join(dest, "plugin.json")
	return copyFile(src, "plugin.json", target)
}

// copyFile copies a file from src FS at srcPath to the destination filesystem path destPath.
func copyFile(src fs.FS, srcPath, destPath string) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer in.Close() //nolint:errcheck

	out, err := os.Create(destPath) //nolint:gosec // G304: destPath is constructed from internal skills dir
	if err != nil {
		return fmt.Errorf("create dest %s: %w", destPath, err)
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", srcPath, err)
	}
	return nil
}

// getPluginNameFromFS reads plugin.json from the embedded FS and extracts the "name" field.
// This is used to determine the correct plugin directory name (e.g., "armature" instead of "claude").
func getPluginNameFromFS(src fs.FS) (string, error) {
	pluginBytes, err := fs.ReadFile(src, "plugin.json")
	if err != nil {
		return "", fmt.Errorf("read plugin.json: %w", err)
	}

	var pluginJSON map[string]any
	if err := json.Unmarshal(pluginBytes, &pluginJSON); err != nil {
		return "", fmt.Errorf("unmarshal plugin.json: %w", err)
	}

	name, ok := pluginJSON["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("plugin.json missing or invalid 'name' field")
	}

	return name, nil
}
