package core

// hierarchy.go — Clojure hierarchy support for isa?/derive/underive.
//
// A hierarchy is a directed acyclic graph (DAG) of parent-child
// relationships between keywords and symbols. The global hierarchy
// is stored as a var and used by default for isa?/derive/underive.

import "sync"

// Hierarchy represents a Clojure hierarchy.
type Hierarchy struct {
	InfoHolder
	MetaHolder
	mu         sync.RWMutex
	parents    map[uint64]map[uint64]bool // child hash → set of parent hashes
	parentKeys map[uint64]Object          // hash → object (for iteration)
	childKeys  map[uint64]Object
}

func MakeHierarchy() *Hierarchy {
	return &Hierarchy{
		parents:    make(map[uint64]map[uint64]bool),
		parentKeys: make(map[uint64]Object),
		childKeys:  make(map[uint64]Object),
	}
}

func (h *Hierarchy) ToString(escape bool) string      { return "#object[Hierarchy]" }
func (h *Hierarchy) Equals(other interface{}) bool    { return h == other }
func (h *Hierarchy) GetType() *Type                   { return TYPE.Fn }
func (h *Hierarchy) Hash() uint32                     { return 0 }
func (h *Hierarchy) WithInfo(info *ObjectInfo) Object { return h }
func (h *Hierarchy) WithMeta(m Map) Object            { return h }

func objKey(obj Object) uint64 {
	return uint64(obj.Hash())<<32 | uint64(obj.Hash())
}

// Derive adds a parent relationship: child isa? parent
func (h *Hierarchy) Derive(child, parent Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if h.parents[ck] == nil {
		h.parents[ck] = make(map[uint64]bool)
	}
	h.parents[ck][pk] = true
	h.parentKeys[pk] = parent
	h.childKeys[ck] = child
}

// Underive removes a parent relationship.
func (h *Hierarchy) Underive(child, parent Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if ps, ok := h.parents[ck]; ok {
		delete(ps, pk)
		if len(ps) == 0 {
			delete(h.parents, ck)
		}
	}
}

// IsA checks if child isa? parent (direct or transitive).
func (h *Hierarchy) IsA(child, parent Object) bool {
	if child.Equals(parent) {
		return true
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.isALocked(objKey(child), objKey(parent), make(map[uint64]bool))
}

func (h *Hierarchy) isALocked(ck, pk uint64, visited map[uint64]bool) bool {
	if visited[ck] {
		return false
	}
	visited[ck] = true

	ps, ok := h.parents[ck]
	if !ok {
		return false
	}
	if ps[pk] {
		return true
	}
	// Transitive check
	for parentKey := range ps {
		if h.isALocked(parentKey, pk, visited) {
			return true
		}
	}
	return false
}

// Parents returns direct parents of tag.
func (h *Hierarchy) Parents(tag Object) []Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tk := objKey(tag)
	ps, ok := h.parents[tk]
	if !ok {
		return nil
	}
	result := make([]Object, 0, len(ps))
	for pk := range ps {
		if obj, ok := h.parentKeys[pk]; ok {
			result = append(result, obj)
		}
	}
	return result
}

// Ancestors returns all transitive ancestors of tag.
func (h *Hierarchy) Ancestors(tag Object) []Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]Object, 0)
	visited := make(map[uint64]bool)
	h.collectAncestors(objKey(tag), &result, visited)
	return result
}

func (h *Hierarchy) collectAncestors(tk uint64, result *[]Object, visited map[uint64]bool) {
	ps, ok := h.parents[tk]
	if !ok {
		return
	}
	for pk := range ps {
		if !visited[pk] {
			visited[pk] = true
			if obj, ok := h.parentKeys[pk]; ok {
				*result = append(*result, obj)
			}
			h.collectAncestors(pk, result, visited)
		}
	}
}

// Descendants returns all transitive descendants of tag.
func (h *Hierarchy) Descendants(tag Object) []Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pk := objKey(tag)
	result := make([]Object, 0)
	visited := make(map[uint64]bool)

	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				result = append(result, obj)
			}
			h.collectDescendants(ck, &result, visited)
		}
	}
	return result
}

func (h *Hierarchy) collectDescendants(pk uint64, result *[]Object, visited map[uint64]bool) {
	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				*result = append(*result, obj)
			}
			h.collectDescendants(ck, result, visited)
		}
	}
}

// Global hierarchy
var globalHierarchy = MakeHierarchy()
