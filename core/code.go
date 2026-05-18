//go:build gen_code
// +build gen_code

// Helpers for gen_code.

package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"reflect"

	gen_go "github.com/rcarmo/go-joker/core/gen/gengo"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

func filenameAsGo(name string) string {
	return corestr.GoName(corestr.FilenameUnbracketed(name))
}

func positionAsGo(filename *string, startLine, startColumn, endLine, endColumn int) string {
	name := ""
	if filename != nil {
		name = filenameAsGo(*filename)
		if name != "" && name != "_" {
			name += "_"
		}
	}
	return fmt.Sprintf("%s%d_%d__%d_%d", name, startLine, startColumn, endLine, endColumn)
}

func isPositionNil(p coretypes.Position) bool {
	return p.EndLine == 0 && p.EndColumn == 0 && p.StartLine == 0 && p.StartColumn == 0 && (p.Filename == nil || *p.Filename == "")
}

func isObjectInfoNil(p *coretypes.ObjectInfo) bool {
	return p == nil || (p.EndLine == 0 && p.EndColumn == 0 && p.StartLine == 0 && p.StartColumn == 0 && (p.Filename == nil || *p.Filename == ""))
}

func symAsGo(sym coretypes.Symbol) string {
	if sym.NameKey() == nil {
		return "EMPTY"
	} else {
		return corestr.SymbolGoName(sym.ToString(false))
	}
}

func fnExprAsGo(f *FnExpr) string {
	return symAsGo(f.self)
}

func (f *FnExpr) AsGo() string {
	name := fmt.Sprintf("fnExpr_POS_%s", positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
	return fmt.Sprintf("%s_NUM_%d", name, ordinalForObj(name, f))
}

func (fn *Fn) AsGo() string {
	if f := fn.fnExpr; f != nil {
		baseName := fmt.Sprintf("fn_%s_POS_%s", fnExprAsGo(f), positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
		return fmt.Sprintf("%s_NUM_%d", baseName, ordinalForObj(baseName, fn))
	}
	panic("(*Fn)Asgo(): fn.fnExpr == nil")
}

func (ns *Namespace) AsGo() string {
	file := ""
	name := ns.Name.Name()
	if ns.Name.Info != nil && ns.Name.Info.Filename != nil && *ns.Name.Info.Filename != name && corestr.FilenameUnbracketed(*ns.Name.Info.Filename) != name {
		file = "_FILE_" + corestr.GoName(*ns.Name.Info.Filename)
	}
	return "ns_" + corestr.GoName(name) + file
}

func (e *Env) AsGo() string {
	if e == GLOBAL_ENV {
		return "global_env"
	}
	panic("not GLOBAL_ENV")
}

func kwAsGo(kw coretypes.Keyword) string {
	return corestr.KeywordGoName(kw.ToString(false))
}

func objectInfoAsGo(oi *coretypes.ObjectInfo) string {
	if res, ok := infoHolderAsGoName(*oi); ok {
		return "objectInfo_" + res
	}
	panic("could not make useful name out of coretypes.ObjectInfo")
}

func (v *Var) AsGo() string {
	sym := v.name
	name := symAsGo(sym)
	ns := ""
	if v.ns != nil {
		nsName := v.ns.Name.Name()
		if symNs := sym.Namespace(); symNs != "" && symNs != nsName {
			msg := fmt.Sprintf("Symbol namespace discrepancy: Var %s has %s, its sym has %s", name, nsName, symNs)
			fmt.Fprintln(Stderr, msg)
			panic(msg)
		}
		if sym.NamespaceKey() == nil {
			i := v.ns.Name.Info
			if i == nil || i.Filename == nil || corestr.FilenameUnbracketed(*i.Filename) != nsName {
				ns = "_NS_" + corestr.GoName(nsName)
			}
		}
	}
	pos := ""
	f := v.Info
	if f == nil {
		f = sym.Info
	}
	if f != nil {
		pos = fmt.Sprintf("_POS_%s", positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
	}
	return "var" + ns + "_NAME_" + name + pos
}

func (v *VarRefExpr) AsGo() string {
	s := v.vr.name.Name()
	if res, ok := infoHolderAsGoName(*v); ok {
		return "varRef_" + corestr.GoName(s) + "_" + res
	}
	return fmt.Sprintf("%s_%d_%d", corestr.VarRefExprName(s), v.StartLine, v.StartColumn)
}

// Returns typename of object as it should be represented in package
// core.
func typeInCore(e interface{}) string {
	return corestr.TypeNameInCore(fmt.Sprintf("%T", e))
}

func typeInCoreAsGo(e interface{}) string {
	return corestr.TypeNameAsGo(typeInCore(e))
}

func infoHolderAsGoName(obj interface{}) (string, bool) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	vt := v.Type()
	sf, yes := vt.FieldByName("InfoHolder")
	if yes {
		if !sf.Anonymous {
			return "", false
		}
		v = v.FieldByName("InfoHolder")
		vt = v.Type()
		if vt.Kind() != reflect.Struct {
			return "", false
		}
		sf, yes = vt.FieldByName("Info")
		if !yes || sf.Anonymous {
			return "", false
		}
		v = v.FieldByName("Info")
		vt = v.Type()
		if vt.Kind() != reflect.Ptr {
			panic("'Info' field not a pointer")
		}
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
		vt = v.Type()
	}
	sf, yes = vt.FieldByName("Position")
	if !yes || !sf.Anonymous {
		return "", false
	}
	v = v.FieldByName("Position")
	vt = v.Type()
	if vt.Kind() != reflect.Struct {
		return "", false
	}
	sf, yes = vt.FieldByName("StartLine")
	if !yes || sf.Anonymous {
		return "", false
	}
	filename := ""
	filenamePtr := gen_go.UnsafeReflectValue(v.FieldByName("Filename"))
	if !(filenamePtr.IsZero() || filenamePtr.IsNil()) {
		filename = filenameAsGo(filenamePtr.Elem().Interface().(string))
		if filename != "" && filename != "_" {
			filename = filename + "_"
		}
	}
	startLine := gen_go.UnsafeReflectValue(v.FieldByName("StartLine")).Interface().(int)
	startColumn := gen_go.UnsafeReflectValue(v.FieldByName("StartColumn")).Interface().(int)
	endLine := gen_go.UnsafeReflectValue(v.FieldByName("EndLine")).Interface().(int)
	endColumn := gen_go.UnsafeReflectValue(v.FieldByName("EndColumn")).Interface().(int)
	return "POS_" + positionAsGo(&filename, startLine, startColumn, endLine, endColumn), true
}

var generatedIds = map[string]*gIdInfo{}

type gIdInfo struct {
	gIds   map[interface{}]uint
	nextId uint
}

func ordinalForObj(id string, obj interface{}) uint {
	info, found := generatedIds[id]
	if !found {
		info = &gIdInfo{map[interface{}]uint{}, 0}
		generatedIds[id] = info
	}
	n, found := info.gIds[obj]
	if !found {
		info.nextId++
		n = info.nextId
		info.gIds[obj] = n
	}
	return n
}

// Tries to call obj.AsGo() and return the result. If that fails,
// cobbles together something reasonable and informative, and returns
// that.
func UniqueId(obj interface{}) (id string) {
	defer func() {
		if r := recover(); r != nil {
			id = typeInCoreAsGo(obj)
			pos, havePos := infoHolderAsGoName(obj)
			if havePos {
				id = fmt.Sprintf("%s_%s", id, pos)
			} else {
				origType := reflect.TypeOf(obj).String()
				if origType == "core.Keyword" || origType == "core.Symbol" {
					fmt.Fprintf(Stderr, "UniqueId: Using %s for %s due to %s\n", id, origType, r)
				}
			}
			n := ordinalForObj(id, obj)
			id = fmt.Sprintf("%s_NUM_%d", id, n)
		}
	}()
	if t, ok := obj.(*coretypes.Type); ok {
		id = "ty_" + corestr.GoName(t.Name)
		return
	}
	id = obj.(interface{ AsGo() string }).AsGo()
	return
}

// Receivers for Joker objects that gen_code.go needs, but no other
// Joker code needs.  (These could be put into object.go, parse.go,
// ns.go, etc., as appropriate, if desired.)

func (v *Var) Expr() Expr {
	return v.expr
}

func (v Var) Namespace() *Namespace {
	return v.ns
}

func (v *VarRefExpr) Var() *Var {
	return v.vr
}
