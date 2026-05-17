package core

import coretypes "github.com/rcarmo/go-joker/core/types"

// z_doc_meta.go — metadata hygiene for native/runtime-installed Vars.
//
// Most public vars are generated from .joke sources and carry rich metadata.
// A few runtime-installed compatibility helpers are installed directly from Go;
// make sure they still have enough metadata for doc generation and lint-style
// checks instead of surfacing as noisy <internal> warnings.

func init() {
	fillNativeVarMetadata()
}

func fillNativeVarMetadata() {
	if GLOBAL_ENV == nil {
		return
	}
	nsRWMu.RLock()
	namespaces := make([]*Namespace, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		namespaces = append(namespaces, ns)
	}
	nsRWMu.RUnlock()
	for _, ns := range namespaces {
		for _, vr := range ns.mappings {
			if vr == nil || vr.ns != ns || vr.isFake {
				continue
			}
			m, _ := vr.meta.(*ArrayMap)
			if m == nil {
				m = collectionConstruction.NewEmptyArrayMap()
				if vr.meta != nil {
					for it := vr.meta.Iter(); it.HasNext(); {
						p := it.Next()
						m.Add(p.Key, p.Value)
					}
				}
				vr.meta = m
			}
			if ok, _ := m.Get(KEYWORDS.ns); !ok {
				m.Add(KEYWORDS.ns, ns)
			}
			if ok, _ := m.Get(KEYWORDS.name); !ok {
				m.Add(KEYWORDS.name, vr.name)
			}
			if vr.isPrivate {
				if ok, _ := m.Get(KEYWORDS.private); !ok {
					m.Add(KEYWORDS.private, coretypes.Boolean{B: true})
				}
				continue
			}
			if ok, _ := m.Get(KEYWORDS.added); !ok {
				m.Add(KEYWORDS.added, coretypes.MakeString("1.0"))
			}
			if ok, _ := m.Get(KEYWORDS.doc); !ok {
				m.Add(KEYWORDS.doc, coretypes.MakeString("Native runtime helper installed by go-joker."))
			}
		}
	}
}
