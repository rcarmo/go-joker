package core

// hierarchy.go — Clojure hierarchy support for isa?/derive/underive.
//
// A hierarchy is a directed acyclic graph (DAG) of parent-child
// relationships between keywords and symbols. The global hierarchy
// is stored as a var and used by default for isa?/derive/underive.

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

// Hierarchy represents a Clojure hierarchy.
type Hierarchy struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	mu         sync.RWMutex
	parents    map[string]map[string]bool  // child key → set of parent keys
	parentKeys map[string]coretypes.Object // key → object (for iteration)
	childKeys  map[string]coretypes.Object
}

func MakeHierarchy() *Hierarchy {
	return &Hierarchy{
		parents:    make(map[string]map[string]bool),
		parentKeys: make(map[string]coretypes.Object),
		childKeys:  make(map[string]coretypes.Object),
	}
}

func (h *Hierarchy) ToString(escape bool) string   { return "#object[Hierarchy]" }
func (h *Hierarchy) Equals(other interface{}) bool { return h == other }
func (h *Hierarchy) GetType() *coretypes.Type      { return TYPE.Fn }
func (h *Hierarchy) Hash() uint32                  { return hashutil.Ptr(uintptr(unsafe.Pointer(h))) }
func (h *Hierarchy) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	h.Info = info
	return h
}
func (h *Hierarchy) WithMeta(m coretypes.Map) coretypes.Object {
	h.Meta = coretypes.SafeMerge(h.Meta, m)
	return h
}

func objKey(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.GetType().ToString(false) + "|" + obj.ToString(false)
}

// Derive adds a parent relationship: child isa? parent
func (h *Hierarchy) Derive(child, parent coretypes.Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if h.parents[ck] == nil {
		h.parents[ck] = make(map[string]bool)
	}
	h.parents[ck][pk] = true
	h.parentKeys[pk] = parent
	h.childKeys[ck] = child
}

// Underive removes a parent relationship.
func (h *Hierarchy) Underive(child, parent coretypes.Object) {
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
func (h *Hierarchy) IsA(child, parent coretypes.Object) bool {
	if child.Equals(parent) {
		return true
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.isALocked(objKey(child), objKey(parent), make(map[string]bool))
}

func (h *Hierarchy) isALocked(ck, pk string, visited map[string]bool) bool {
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
func (h *Hierarchy) Parents(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tk := objKey(tag)
	ps, ok := h.parents[tk]
	if !ok {
		return nil
	}
	result := make([]coretypes.Object, 0, len(ps))
	for pk := range ps {
		if obj, ok := h.parentKeys[pk]; ok {
			result = append(result, obj)
		}
	}
	return result
}

// Ancestors returns all transitive ancestors of tag.
func (h *Hierarchy) Ancestors(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)
	h.collectAncestors(objKey(tag), &result, visited)
	return result
}

func (h *Hierarchy) collectAncestors(tk string, result *[]coretypes.Object, visited map[string]bool) {
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
func (h *Hierarchy) Descendants(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pk := objKey(tag)
	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)

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

func (h *Hierarchy) collectDescendants(pk string, result *[]coretypes.Object, visited map[string]bool) {
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

// ---- hierarchy_init.go ----
// hierarchy_init.go — Register derive, underive, isa?, ancestors, descendants, parents, make-hierarchy.

func init() {
	registerHierarchyProcs()
}

func registerHierarchyProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// make-hierarchy
	mhVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"))
	mhVr.Value = Proc{Name: "procMakeHierarchy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 0, 0)
		return MakeHierarchy()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"), mhVr)

	// derive — (derive child parent) or (derive h child parent)
	deriveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "derive"))
	deriveVr.Value = Proc{Name: "procDerive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Derive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity derive must be a hierarchy"))
			}
			h.Derive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "derive"), deriveVr)

	// underive — (underive child parent) or (underive h child parent)
	underiveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "underive"))
	underiveVr.Value = Proc{Name: "procUnderive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Underive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity underive must be a hierarchy"))
			}
			h.Underive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "underive"), underiveVr)

	// isa? — (isa? child parent) or (isa? h child parent)
	isaVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "isa?"))
	isaVr.Value = Proc{Name: "procIsaQ", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			return coretypes.MakeBoolean(globalHierarchy.IsA(args[0], args[1]))
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity isa? must be a hierarchy"))
			}
			return coretypes.MakeBoolean(h.IsA(args[1], args[2]))
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "isa?"), isaVr)

	// parents — (parents tag) or (parents h tag)
	parentsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "parents"))
	parentsVr.Value = Proc{Name: "procParents", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity parents must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ps := h.Parents(tag)
		if len(ps) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, p := range ps {
			s = s.Conj(p).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "parents"), parentsVr)

	// ancestors — (ancestors tag) or (ancestors h tag)
	ancestorsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"))
	ancestorsVr.Value = Proc{Name: "procAncestors", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity ancestors must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		as := h.Ancestors(tag)
		if len(as) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, a := range as {
			s = s.Conj(a).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"), ancestorsVr)

	// descendants — (descendants tag) or (descendants h tag)
	descendantsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "descendants"))
	descendantsVr.Value = Proc{Name: "procDescendants", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity descendants must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ds := h.Descendants(tag)
		if len(ds) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, d := range ds {
			s = s.Conj(d).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "descendants"), descendantsVr)
}
