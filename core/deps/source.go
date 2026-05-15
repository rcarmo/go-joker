package deps

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var externalHTTPClient = &http.Client{Timeout: 30 * time.Second}

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
		resp, err := externalHTTPClient.Get(url)
		if err != nil {
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				return "", closeErr
			}
			return "", fmt.Errorf("unable to retrieve: %s\nserver response: %d", url, resp.StatusCode)
		}

		out, err := os.Create(libPath)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(out, resp.Body)
		bodyCloseErr := resp.Body.Close()
		outCloseErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if bodyCloseErr != nil {
			return "", bodyCloseErr
		}
		if outCloseErr != nil {
			return "", outCloseErr
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
