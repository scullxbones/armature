package adapters

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type lookPathConfig struct {
	windows bool
	pathext string
	listSep rune
}

func envVal(environ []string, key string) string {
	return envLookup(environ, key, false)
}

func envValFold(environ []string, key string) string {
	return envLookup(environ, key, true)
}

func envLookup(environ []string, key string, fold bool) string {
	for i := len(environ) - 1; i >= 0; i-- {
		name, val, ok := splitEnvKV(environ[i])
		if !ok {
			continue
		}
		if (fold && strings.EqualFold(name, key)) || (!fold && name == key) {
			return val
		}
	}
	return ""
}

func splitEnvKV(kv string) (name, value string, ok bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}

func lookPathOn(pathList, file string) (string, error) {
	return lookPathOnConfig(pathList, file, lookPathConfig{
		windows: runtime.GOOS == "windows",
		pathext: envValFold(os.Environ(), "PATHEXT"),
	})
}

func lookPathOnConfig(pathList, file string, cfg lookPathConfig) (string, error) {
	exts := unixExts
	if cfg.windows {
		exts = splitPATHEXT(cfg.pathext)
	}
	sep := cfg.listSep
	dirs := filepath.SplitList(pathList)
	if sep != 0 {
		dirs = strings.Split(pathList, string(sep))
	}
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		found, err := findExecutable(filepath.Join(dir, file), exts, cfg.windows)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(found) {
			return found, &exec.Error{Name: file, Err: exec.ErrDot}
		}
		return found, nil
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

var unixExts = []string{""} //nolint:gochecknoglobals //immutable lookpath extension list

func splitPATHEXT(pathext string) []string {
	if pathext == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var exts []string
	for e := range strings.SplitSeq(strings.ToLower(pathext), ";") {
		if e == "" {
			continue
		}
		if e[0] != '.' {
			e = "." + e
		}
		exts = append(exts, e)
	}
	if len(exts) == 0 {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	return exts
}

func findExecutable(file string, exts []string, windows bool) (string, error) {
	if !windows {
		st, err := os.Stat(file)
		if err != nil || st.IsDir() {
			return "", fs.ErrNotExist
		}
		if st.Mode()&0o111 == 0 {
			return "", fs.ErrPermission
		}
		return file, nil
	}
	if hasWindowsExt(file) {
		if err := chkStat(file); err == nil {
			return file, nil
		}
	}
	for _, e := range exts {
		if err := chkStat(file + e); err == nil {
			return file + e, nil
		}
	}
	if err := chkStat(file); err == nil {
		return file, nil
	}
	return "", fs.ErrNotExist
}

func hasWindowsExt(file string) bool {
	i := strings.LastIndex(file, ".")
	if i < 0 {
		return false
	}
	return strings.LastIndexAny(file, `:\/`) < i
}

func chkStat(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if d.IsDir() {
		return fs.ErrPermission
	}
	return nil
}
