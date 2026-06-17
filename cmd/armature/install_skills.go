package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)

func newInstallSkillsCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "install-skills",
		Short: "Deploy bundled skills to .claude/skills/ and .claude/plugins/armature/",
		Long: "Copies the embedded skills to .claude/skills/ (local) or ~/.claude/skills/ (--global)." +
			" Also writes a flat <name>.md file for each skill so the Skill tool can load them by name," +
			" and deploys plugin.json to .claude/plugins/armature/ for the Skill tool registry.",
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

			skillsDest := filepath.Join(destBase, ".claude", "skills")
			if err := deploySkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return err
			}
			// Write flat <name>.md files so the Skill tool can load skills by name.
			if err := deployFlatSkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skills deployed to %s\n", skillsDest)

			// Deploy plugin.json to .claude/plugins/armature for Skill tool registry
			pluginsDest := filepath.Join(destBase, ".claude", "plugins", "armature")
			if err := deployPlugin(skillsembed.SkillsFS, pluginsDest); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plugin configuration deployed to %s\n", pluginsDest)
			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "install to ~/.claude/skills/ instead of .claude/skills/")
	return cmd
}
