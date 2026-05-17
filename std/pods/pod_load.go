package pods

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

const podDescribeTimeout = 5 * time.Second

func loadPod(args []Object) Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("pods/load-pod expects command path or pod name plus optional version/args"))
	}
	cmdPath := EnsureArgIsString(args, 0).S
	cmdArgs := []string{}
	if len(args) == 2 {
		switch v := args[1].(type) {
		case coretypes.String:
			var err error
			cmdPath, err = findCachedPod(cmdPath, v.S)
			if err != nil {
				panic(RT.NewError("pods/load-pod: " + err.Error()))
			}
		case Seqable:
			for s := v.Seq(); !s.IsEmpty(); s = s.Rest() {
				cmdArgs = append(cmdArgs, EnsureObjectIsString(s.First(), "pods/load-pod args must be strings").S)
			}
		default:
			panic(RT.NewError("pods/load-pod second argument must be version string or seq of command args"))
		}
	}
	p, describe, err := startPodProcess(cmdPath, cmdArgs)
	if err != nil {
		panic(RT.NewError("pods/load-pod: " + err.Error()))
	}
	if format, ok := describe["format"].(string); ok && format != "" {
		p.format = format
	}
	if name := firstPodNamespaceName(describe); name != "" {
		p.name = name
	}
	registerPod(p)
	if err := installPodDescribeNamespaces(p, describe); err != nil {
		p.shutdownPod()
		panic(RT.NewError("pods/load-pod: " + err.Error()))
	}
	p.startRouter()
	return coretypes.MakeString(p.id)
}

func startPodProcess(cmdPath string, args []string) (*Pod, podMessage, error) {
	cmd := exec.Command(cmdPath, args...)
	if os.Getenv("GO_WANT_FAKE_POD") == "1" {
		cmd.Env = append(os.Environ(), "GO_WANT_FAKE_POD=1")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	id := "pod-" + filepath.Base(cmdPath) + "-" + fmt.Sprint(cmd.Process.Pid)
	p := newPod(id, filepath.Base(cmdPath), "json", stdin, stdout, stderr)
	p.cmd = cmd
	descID := p.nextID()
	ch := p.registerPending(descID)
	p.startRouter()
	if err := p.send(podMessage{"op": "describe", "id": descID}); err != nil {
		p.shutdownPod()
		return nil, nil, err
	}
	select {
	case msg, ok := <-ch:
		if !ok {
			p.shutdownPod()
			return nil, nil, fmt.Errorf("pod closed before describe response")
		}
		return p, msg, nil
	case <-time.After(podDescribeTimeout):
		p.shutdownPod()
		return nil, nil, fmt.Errorf("timed out waiting for describe response")
	}
}

func (p *Pod) send(msg podMessage) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err := p.stdin.Write(bencodeEncodePlain(msg))
	return err
}

func firstPodNamespaceName(msg podMessage) string {
	nss, ok := msg["namespaces"].([]any)
	if !ok || len(nss) == 0 {
		return ""
	}
	switch ns := nss[0].(type) {
	case map[string]any:
		if name, ok := ns["name"].(string); ok {
			return name
		}
	case podMessage:
		if name, ok := ns["name"].(string); ok {
			return name
		}
	}
	return ""
}

func findCachedPod(name, version string) (string, error) {
	podsDir := os.Getenv("BABASHKA_PODS_DIR")
	if podsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		podsDir = filepath.Join(home, ".babashka", "pods")
	}
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	repoDir := filepath.Join(podsDir, "repository", name, version, osName, arch)
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return "", fmt.Errorf("pod %s@%s not found in cache (%s)", name, version, repoDir)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(repoDir, e.Name())
		if st, err := os.Stat(path); err == nil && st.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("pod %s@%s cache has no executable in %s", name, version, repoDir)
}
