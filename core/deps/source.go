package deps

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func ExternalHTTPSourceToPath(home, lib, url string) (string, error) {
	localBase := filepath.Join(home, ".jokerd", "deps", strings.SplitN(url, "//", 2)[1])
	libBase := LibNamePath(lib)
	libPath := filepath.Join(localBase, libBase)
	libPathDir := filepath.Dir(libPath)

	if err := os.MkdirAll(libPathDir, 0o777); err != nil {
		return "", err
	}

	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		if !strings.HasSuffix(url, ".joke") {
			url = url + libBase
		}
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("unable to retrieve: %s\nserver response: %d", url, resp.StatusCode)
		}

		out, err := os.Create(libPath)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(out, resp.Body)
		closeErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}

	return libPath, nil
}

func ExternalSourceToPath(home, lib, url string) (string, error) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return ExternalHTTPSourceToPath(home, lib, url)
	}
	return ResolveLibPath(url, lib), nil
}
