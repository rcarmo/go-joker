package runtime

import "testing"

func TestNamespaceMuIsUsable(t *testing.T) {
	NamespaceMu.Lock()
	NamespaceMu.Unlock()
	NamespaceMu.RLock()
	NamespaceMu.RUnlock()
}
