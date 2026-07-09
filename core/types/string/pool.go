package string

import "sync"

type Pool map[string]*string

// poolMu protects all Pool instances. Pool remains a map type because generated
// bootstrap payloads initialize it with map literals.
var poolMu sync.RWMutex

func (p Pool) Intern(s string) *string {
	poolMu.RLock()
	ss := p[s]
	poolMu.RUnlock()
	if ss != nil {
		return ss
	}

	poolMu.Lock()
	defer poolMu.Unlock()
	if ss = p[s]; ss != nil {
		return ss
	}
	p[s] = &s
	return &s
}
