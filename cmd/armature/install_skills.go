package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)

func newInstallSkillsCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:               "install-skills",
		Short:             "Deploy bundled skills to .claude/skills/",
		Long:              "Copies the embedded skills to .claude/skills/ (local) or ~/.claude/skills/ (--global).",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			var destBase string
			if global {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home directory: %w", err)
				}
				destBase = home
			} else {
				repoPath, _ := cmd.Flags().GetString("repo")
				if repoPath == "" {
					repoPath = "."
				}
				absRepo, err := filepath.Abs(repoPath)
				if err != nil {
					return fmt.Errorf("resolve repo path: %w", err)
				}
				destBase = absRepo
			}

			dest := filepath.Join(destBase, ".claude", "skills")
			if err := deploySkills(skillsembed.SkillsFS, dest); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skills deployed to %s\n", dest)
			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "install to ~/.claude/skills/ instead of .claude/skills/")
	return cmd
}

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
			return os.MkdirAll(target, 0755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}

		return copyFile(src, path, target)
	})
}

func copyFile(src fs.FS, srcPath, destPath string) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer in.Close() //nolint:errcheck

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest %s: %w", destPath, err)
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", srcPath, err)
	}
	return nil
}
