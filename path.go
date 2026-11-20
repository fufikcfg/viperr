package viperr

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// configMarkers are the files that identify an application directory.
var configMarkers = []string{
	".env",
	"settings.xml",
	filepath.Join("settings", "settings.xml"),
}

var foreignWindowsAbs = regexp.MustCompile(`^[A-Za-z]:[\\/]+(.*)$`)

// Resolve joins a configuration path to base.
//
// A native absolute path is cleaned and returned. On Windows a path with a
// drive letter is absolute; a path that only starts with / or \ is relative to
// base. On other systems a Windows path such as C:\logs is treated as relative,
// and the drive prefix is removed.
//
// An empty path stays empty. The caller decides the fallback, usually with default.
func Resolve(path, base string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if runtime.GOOS != "windows" {
		if m := foreignWindowsAbs.FindStringSubmatch(path); m != nil {
			path = m[1]
		} else if nativeAbs(path) {
			return filepath.Clean(path)
		}
	} else if nativeAbs(path) {
		return filepath.Clean(path)
	}
	path = strings.TrimLeft(path, `\/`)
	path = filepath.FromSlash(strings.ReplaceAll(path, `\`, `/`))
	if path == "" {
		return ""
	}
	if strings.TrimSpace(base) == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func nativeAbs(path string) bool {
	if runtime.GOOS == "windows" {
		return filepath.VolumeName(path) != "" && filepath.IsAbs(path)
	}
	return filepath.IsAbs(path)
}

// ResolveAppDir returns the first of the working directory, the executable
// directory, and the executable parent that contains .env, settings.xml, or
// settings/settings.xml. If none do, it returns the working directory, then
// the executable directory.
func ResolveAppDir() (string, error) {
	wd, wdErr := os.Getwd()
	exe, exeErr := os.Executable()
	var exeDir, parent string
	if exeErr == nil {
		exeDir = filepath.Dir(exe)
		parent = filepath.Dir(exeDir)
	}

	var candidates []string
	if wdErr == nil && wd != "" {
		candidates = append(candidates, wd)
	}
	if exeDir != "" {
		candidates = append(candidates, exeDir)
	}
	if parent != "" && parent != exeDir {
		candidates = append(candidates, parent)
	}
	seen := map[string]struct{}{}
	for _, dir := range candidates {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		if dirHasConfig(dir) {
			return dir, nil
		}
	}
	switch {
	case wdErr == nil && wd != "":
		return wd, nil
	case exeDir != "":
		return exeDir, nil
	case wdErr != nil:
		return "", fmt.Errorf("viperr: app dir: %w", wdErr)
	default:
		return "", fmt.Errorf("viperr: app dir: executable path is unavailable")
	}
}

func dirHasConfig(dir string) bool {
	for _, name := range configMarkers {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func settingsFile(dir string) (string, error) {
	direct := filepath.Join(dir, "settings.xml")
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return direct, nil
	}
	nested := filepath.Join(dir, "settings", "settings.xml")
	if info, err := os.Stat(nested); err == nil && !info.IsDir() {
		return nested, nil
	}
	return "", os.ErrNotExist
}
