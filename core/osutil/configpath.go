package osutil

import (
	"os"
	"path/filepath"
)

// Abs wraps filepath.Abs for shared path normalization.
func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// FindConfigPath searches upward from filename/workingDir for configName and
// falls back to home/configName. When findDir is true, only directory matches
// are accepted.
func FindConfigPath(filename, workingDir, configName, home string, findDir bool) (string, error) {
	var err error
	if filename != "" {
		filename, err = Abs(filename)
		if err != nil {
			return "", err
		}
	}
	if workingDir != "" {
		workingDir, err = Abs(workingDir)
		if err != nil {
			return "", err
		}
		filename = filepath.Join(workingDir, configName)
	}
	for {
		oldFilename := filename
		filename = filepath.Dir(filename)
		if filename == oldFilename {
			if home == "" {
				return "", nil
			}
			p := filepath.Join(home, configName)
			if info, err := os.Stat(p); err == nil {
				if !findDir || info.IsDir() {
					return p, nil
				}
			}
			return "", nil
		}
		p := filepath.Join(filename, configName)
		if info, err := os.Stat(p); err == nil {
			if !findDir || info.IsDir() {
				return p, nil
			}
		}
	}
}
