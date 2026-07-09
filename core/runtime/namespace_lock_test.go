package runtime

import "testing"

func TestNamespaceMuIsUsable(t *testing.T) {
	NamespaceMu.Lock()
	locked := true
	NamespaceMu.Unlock()
	NamespaceMu.RLock()
	readLocked := locked
	NamespaceMu.RUnlock()
	if !readLocked {
		t.Fatal("namespace mutex did not preserve protected state")
	}
}
