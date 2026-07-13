// Package platform reads and writes git-config-backed values, such as the worker identity used by claim and heartbeat.
package platform

import "github.com/scullxbones/armature/internal/adapters"

// GitConfigPort abstracts local git config access for a single repository.
type GitConfigPort interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

type repoGitConfig struct {
	client *adapters.Client
}

// NewGitConfigPort returns a repository-scoped git config adapter.
func NewGitConfigPort(repoPath string) GitConfigPort {
	return &repoGitConfig{client: adapters.New(repoPath)}
}

func (r *repoGitConfig) Get(key string) (string, error) {
	return r.client.ReadGitConfig(key)
}

func (r *repoGitConfig) Set(key, value string) error {
	return r.client.SetGitConfig(key, value)
}
