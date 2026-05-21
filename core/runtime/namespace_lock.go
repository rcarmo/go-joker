package runtime

import "sync"

// NamespaceMu protects root runtime namespace map mutations until namespace/env
// ownership moves out of root core as a coherent batch.
var NamespaceMu sync.RWMutex
