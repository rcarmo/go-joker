package osutil

import "os"

func HomeDir() string {
	home, ok := os.LookupEnv("HOME")
	if !ok {
		home, _ = os.LookupEnv("USERPROFILE")
	}
	return home
}
