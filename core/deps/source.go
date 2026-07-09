package deps

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxExternalSourceBytes = 32 << 20

func externalURLCacheBase(rawURL string) (string, error) {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid external URL: %s", rawURL)
	}
	hash := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%s-%x", parsed.Hostname(), hash[:8]), nil
}

func safeCachePath(base, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid dependency path: %s", relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid dependency path: %s", relative)
	}
	path := filepath.Join(base, clean)
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dependency path escapes cache: %s", relative)
	}
	return path, nil
}

var externalHTTPClient = &http.Client{Timeout: 30 * time.Second}

func ExternalHTTPSourceToPath(home, lib, rawURL string) (string, error) {
	cacheBase, err := externalURLCacheBase(rawURL)
	if err != nil {
		return "", err
	}
	localBase := filepath.Join(home, ".jokerd", "deps", cacheBase)
	libBase := LibNamePath(lib)
	libPath, err := safeCachePath(localBase, libBase)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		return "", err
	}

	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		requestURL := rawURL
		if !strings.HasSuffix(requestURL, ".joke") {
			requestURL += libBase
		}
		resp, err := externalHTTPClient.Get(requestURL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("unable to retrieve: %s\nserver response: %d", requestURL, resp.StatusCode)
		}

		tmp, err := os.CreateTemp(filepath.Dir(libPath), ".dependency-*.tmp")
		if err != nil {
			return "", err
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)

		written, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, maxExternalSourceBytes+1))
		closeErr := tmp.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if written > maxExternalSourceBytes {
			return "", fmt.Errorf("external source exceeds %d bytes", maxExternalSourceBytes)
		}
		if err := os.Chmod(tmpPath, 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmpPath, libPath); err != nil {
			if _, statErr := os.Stat(libPath); statErr != nil {
				return "", err
			}
		}
	}

	return libPath, nil
}

func ExternalSourceToPath(home, lib, sourceURL string) (string, error) {
	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		return ExternalHTTPSourceToPath(home, lib, sourceURL)
	}
	return ResolveLibPath(sourceURL, lib), nil
}
