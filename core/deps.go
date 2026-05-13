package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rcarmo/go-joker/core/osutil"
)

func externalHttpSourceToPath(lib string, url string) (path string) {
	home := osutil.HomeDir()
	localBase := filepath.Join(home, ".jokerd", "deps", strings.SplitN(url, "//", 2)[1])
	libBase := filepath.Join(strings.Split(lib, ".")...) + ".joke"
	libPath := filepath.Join(localBase, libBase)
	libPathDir := filepath.Dir(libPath)

	if _, err := os.Stat(libPathDir); os.IsNotExist(err) {
		PanicOnErr(os.MkdirAll(libPathDir, 0o777))
	}

	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		if !strings.HasSuffix(url, ".joke") {
			url = url + libBase
		}
		resp, err := http.Get(url)
		PanicOnErr(err)

		if resp.StatusCode != http.StatusOK {
			closeErr := resp.Body.Close()
			PanicOnErr(closeErr)
			panic(RT.NewError(fmt.Sprintf("Unable to retrieve: %s\nServer response: %d", url, resp.StatusCode)))
		}

		out, err := os.Create(libPath)
		PanicOnErr(err)

		_, err = io.Copy(out, resp.Body)
		bodyCloseErr := resp.Body.Close()
		fileCloseErr := out.Close()
		PanicOnErr(err)
		PanicOnErr(bodyCloseErr)
		PanicOnErr(fileCloseErr)
	}

	return libPath
}

func externalSourceToPath(lib string, url string) (path string) {
	httpPath, _ := regexp.MatchString("http://|https://", url)
	if httpPath {
		return externalHttpSourceToPath(lib, url)
	} else {
		return filepath.Join(append([]string{url}, strings.Split(lib, ".")...)...) + ".joke"
	}
}
