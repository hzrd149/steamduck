package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// windowPIDs finds the processes of a kwakore window: the kwaklet child
// carries KWAKORE_INSTANCE_ID=<window_id> in its environment, and WebKit's
// helper processes it starts inherit that environment.
func windowPIDs(procRoot, windowID string) map[int]bool {
	pids := map[int]bool{}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return pids
	}
	needle := []byte("KWAKORE_INSTANCE_ID=" + windowID + "\x00")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		env, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "environ"))
		if err != nil {
			continue
		}
		if bytes.HasPrefix(env, needle) || bytes.Contains(env, append([]byte{0}, needle...)) {
			pids[pid] = true
		}
	}
	return pids
}

// parseEnviron splits a NUL-separated environment block.
func parseEnviron(b []byte) map[string]string {
	env := map[string]string{}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if k, v, ok := strings.Cut(string(kv), "="); ok {
			env[k] = v
		}
	}
	return env
}
