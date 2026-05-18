package core

import coretypes "github.com/rcarmo/go-joker/core/types"

func (expr *LiteralExpr) InferType() *coretypes.Type {
	if expr.isSurrogate {
		return nil
	}
	return expr.obj.GetType()
}

func dumpPosition(p coretypes.Position) coretypes.Map {
	res := collectionConstruction.NewEmptyArrayMap()
	res.Add(KEYWORDS.startLine, coretypes.Int{I: p.StartLine})
	res.Add(KEYWORDS.endLine, coretypes.Int{I: p.EndLine})
	res.Add(KEYWORDS.startColumn, coretypes.Int{I: p.StartColumn})
	res.Add(KEYWORDS.endColumn, coretypes.Int{I: p.EndColumn})
	res.Add(KEYWORDS.filename, coretypes.String{S: p.FilenameOrUnknown()})
	return res
}

func exprArrayMap(expr Expr, exprType string, pos bool) *ArrayMap {
	res := collectionConstruction.NewEmptyArrayMap()
	res.Add(KEYWORDS.type_, coretypes.MakeKeyword(STRINGS.Intern, exprType))
	if pos {
		res.Add(KEYWORDS.pos, dumpPosition(expr.Pos()))
	}
	return res
}

func addVector(res *ArrayMap, body []Expr, name string, pos bool) {
	b := collectionConstruction.NewEmptyVector()
	for _, e := range body {
		b = b.Conjoin(e.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, name), b)
}

func (expr *LiteralExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "literal", pos)
	res.Add(KEYWORDS.object, expr.obj)
	return res
}

func (expr *VectorExpr) InferType() *coretypes.Type {
	return TYPE.Vec
}

func (expr *VectorExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "vector", pos)
	addVector(res, expr.v, "vector", pos)
	return res
}

func (expr *MapExpr) InferType() *coretypes.Type {
	return TYPE.ArrayMap
}

func (expr *MapExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "map", pos)
	addVector(res, expr.keys, "keys", pos)
	addVector(res, expr.values, "values", pos)
	return res
}

func (expr *SetExpr) InferType() *coretypes.Type {
	return TYPE.MapSet
}

func (expr *SetExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "set", pos)
	addVector(res, expr.elements, "set", pos)
	return res
}

func (expr *IfExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *IfExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "if", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "condition"), expr.cond.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "positive"), expr.positive.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "negative"), expr.negative.Dump(pos))
	return res
}

func (expr *DefExpr) InferType() *coretypes.Type {
	return TYPE.Var
}

func (expr *DefExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "def", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	res.Add(KEYWORDS.name, expr.name)
	if expr.value != nil {
		res.Add(KEYWORDS.value, expr.value.Dump(pos))
	}
	if expr.meta != nil {
		res.Add(KEYWORDS.meta, expr.meta.Dump(pos))
	}
	return res
}

func (expr *CallExpr) InferType() *coretypes.Type {
	switch callableExpr := expr.callable.(type) {
	case *VarRefExpr:
		switch f := callableExpr.vr.Value.(type) {
		case *Fn:
			if arity := selectArity(f.fnExpr, len(expr.args)); arity != nil && arity.taggedType != nil {
				return arity.taggedType
			}
		}
		return callableExpr.vr.taggedType
	}
	return nil
}

func (expr *CallExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "call", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.String{S: expr.Name()})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "callable"), expr.callable.Dump(pos))
	addVector(res, expr.args, "args", pos)
	return res
}

func (expr *MacroCallExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *MacroCallExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "macro-call", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.String{S: expr.name})
	args := collectionConstruction.NewEmptyVector()
	for _, arg := range expr.args {
		args = args.Conjoin(arg)
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "args"), args)
	return res
}

func (expr *RecurExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *RecurExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "recur", pos)
	addVector(res, expr.args, "args", pos)
	return res
}

func (expr *VarRefExpr) InferType() *coretypes.Type {
	// if expr.vr.taggedType != nil {
	// 	return expr.vr.taggedType
	// }
	if expr.vr.expr == nil {
		return nil
	}
	if expr.vr.isDynamic {
		return nil
	}
	return expr.vr.expr.InferType()
}

func (expr *VarRefExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "var-ref", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	return res
}

func (expr *SetMacroExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *SetMacroExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "set-macro", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	return res
}

func (expr *BindingExpr) InferType() *coretypes.Type {
	return expr.binding.inferredType
}

func (expr *BindingExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "binding", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), expr.binding.name)
	return res
}

func (expr *MetaExpr) InferType() *coretypes.Type {
	return expr.expr.InferType()
}

func (expr *MetaExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "meta", pos)
	res.Add(KEYWORDS.meta, expr.meta.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "expr"), expr.expr.Dump(pos))
	return res
}

func typeOfLast(exprs []Expr) *coretypes.Type {
	n := len(exprs)
	if n > 0 {
		return exprs[n-1].InferType()
	}
	return nil
}

func (expr *DoExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *DoExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "do", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *FnExpr) InferType() *coretypes.Type {
	return TYPE.Fn
}

func (expr *FnArityExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *FnArityExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "arity", pos)
	args := collectionConstruction.NewEmptyVector()
	for _, arg := range expr.args {
		args = args.Conjoin(arg)
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "args"), args)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *FnExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "fn", pos)
	if expr.self.NameKey() != nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "self"), expr.self)
	}
	if expr.variadic != nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "variadic"), expr.variadic.Dump(pos))
	}
	arities := collectionConstruction.NewEmptyVector()
	for _, a := range expr.arities {
		arities = arities.Conjoin(a.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "arities"), arities)
	return res
}

func (expr *LetExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *LetExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "let", pos)
	names := collectionConstruction.NewEmptyVector()
	for _, name := range expr.names {
		names = names.Conjoin(name)
	}
	addVector(res, expr.values, "values", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *LoopExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *LoopExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "loop", pos)
	names := collectionConstruction.NewEmptyVector()
	for _, name := range expr.names {
		names = names.Conjoin(name)
	}
	addVector(res, expr.values, "values", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *ThrowExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *ThrowExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "throw", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "expr"), expr.e.Dump(pos))
	return res
}

func (expr *CatchExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *CatchExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "catch", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "error-type"), expr.excType)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "error-symbol"), expr.excSymbol)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *TryExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *TryExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "try", pos)
	addVector(res, expr.body, "body", pos)
	addVector(res, expr.finallyExpr, "finally", pos)
	catches := collectionConstruction.NewEmptyVector()
	for _, c := range expr.catches {
		catches = catches.Conjoin(c.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "catches"), catches)
	return res
}
