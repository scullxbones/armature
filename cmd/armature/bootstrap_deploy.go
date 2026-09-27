package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func deploySkills(src fs.FS, dest string) error {
	sub, err := fs.Sub(src, "skills")
	if err != nil {
		return fmt.Errorf("skills subtree: %w", err)
	}
	return deploySkillsRoot(sub, dest)
}

func deployFlatSkills(src fs.FS, dest string) error {
	sub, err := fs.Sub(src, "skills")
	if err != nil {
		return fmt.Errorf("skills subtree: %w", err)
	}
	return deployFlatSkillsRoot(sub, dest)
}

func deployRepoAgentSkills(repoPath, dest string) error {
	srcDir := filepath.Join(repoPath, ".agents", "skills")
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", srcDir, err)
	}
	if !info.IsDir() {
		return nil
	}
	src := os.DirFS(srcDir)
	if err := deploySkillsRoot(src, dest); err != nil {
		return fmt.Errorf("deploy repo agent skills: %w", err)
	}
	if err := deployFlatSkillsRoot(src, dest); err != nil {
		return fmt.Errorf("deploy flat repo agent skills: %w", err)
	}
	return nil
}

func deploySkillsRoot(src fs.FS, dest string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return os.MkdirAll(dest, 0o750)
		}

		target := filepath.Join(dest, filepath.FromSlash(path))

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}

		return copyFile(src, path, target)
	})
}

func deployFlatSkillsRoot(src fs.FS, dest string) error {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return fmt.Errorf("read skills root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillFile := name + "/SKILL.md"
		if _, err := fs.Stat(src, skillFile); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat skill %s: %w", name, err)
		}
		target := filepath.Join(dest, name+".md")
		if err := copySkillWithRewrittenRefs(src, skillFile, name, target); err != nil {
			return fmt.Errorf("deploy flat skill %s: %w", name, err)
		}
	}
	return nil
}

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

func deployPlugin(src fs.FS, dest string) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create plugin directory %s: %w", dest, err)
	}

	target := filepath.Join(dest, "plugin.json")
	return copyFile(src, "plugin.json", target)
}

func copyFile(src fs.FS, srcPath, destPath string) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer bestEffortClose(in)

	out, err := os.Create(destPath) //nolint:gosec // G304: destPath is constructed from internal skills dir
	if err != nil {
		return fmt.Errorf("create dest %s: %w", destPath, err)
	}
	defer bestEffortClose(out)

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", srcPath, err)
	}
	return nil
}

func pluginNameFromSkillsFS(src fs.FS) (string, error) {
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
