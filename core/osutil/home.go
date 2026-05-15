package osutil

import "os"

func HomeDir() string {
	if home, ok := os.LookupEnv("HOME"); ok && home != "" {
		return home
	}
	if home, ok := os.LookupEnv("USERPROFILE"); ok && home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
