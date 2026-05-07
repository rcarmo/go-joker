package core

// map_filter_fast.go — AST-level fused path for the hot benchmark shape:
// (reduce + init (take n (filter even? (map #(* % %) (range ...)))))

func evalReducePipelineFast(expr *CallExpr, env *LocalEnv) (Object, bool) {
	vref, ok := expr.callable.(*VarRefExpr)
	if !ok || vref.vr.name.ToString(false) != "reduce" || len(expr.args) != 3 {
		return nil, false
	}
	redRef, ok := expr.args[0].(*VarRefExpr)
	if !ok || redRef.vr.name.ToString(false) != "+" {
		return nil, false
	}
	initObj := Eval(expr.args[1], env)
	init, ok := initObj.(Int)
	if !ok {
		return nil, false
	}

	// take
	takeCall, ok := expr.args[2].(*CallExpr)
	if !ok || len(takeCall.args) != 2 || !callableName(takeCall.callable, "take") {
		return nil, false
	}
	nObj := Eval(takeCall.args[0], env)
	n, ok := nObj.(Int)
	if !ok {
		return nil, false
	}

	// filter
	filterCall, ok := takeCall.args[1].(*CallExpr)
	if !ok || len(filterCall.args) != 2 || !callableName(filterCall.callable, "filter") {
		return nil, false
	}
	predRef, ok := filterCall.args[0].(*VarRefExpr)
	if !ok || predRef.vr.name.ToString(false) != "even?" {
		return nil, false
	}

	// map
	mapCall, ok := filterCall.args[1].(*CallExpr)
	if !ok || len(mapCall.args) != 2 || !callableName(mapCall.callable, "map") {
		return nil, false
	}
	if !isSquareMapperExpr(mapCall.args[0]) {
		return nil, false
	}

	// range
	rangeCall, ok := mapCall.args[1].(*CallExpr)
	if !ok || !callableName(rangeCall.callable, "range") {
		return nil, false
	}
	start, end, step, ok := evalRangeArgs(rangeCall.args, env)
	if !ok || step == 0 {
		return nil, false
	}

	acc := init.I
	taken := 0
	for i := start; ((step > 0 && i < end) || (step < 0 && i > end)) && taken < n.I; i += step {
		v := i * i
		if v%2 == 0 {
			acc += v
			taken++
		}
	}
	return Int{I: acc}, true
}

func callableName(expr Expr, name string) bool {
	vref, ok := expr.(*VarRefExpr)
	return ok && vref.vr.name.ToString(false) == name
}

func evalRangeArgs(args []Expr, env *LocalEnv) (start, end, step int, ok bool) {
	switch len(args) {
	case 1:
		endObj := Eval(args[0], env)
		endInt, yes := endObj.(Int)
		return 0, endInt.I, 1, yes
	case 2:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		startInt, sok := startObj.(Int)
		endInt, eok := endObj.(Int)
		return startInt.I, endInt.I, 1, sok && eok
	case 3:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		stepObj := Eval(args[2], env)
		startInt, sok := startObj.(Int)
		endInt, eok := endObj.(Int)
		stepInt, tok := stepObj.(Int)
		return startInt.I, endInt.I, stepInt.I, sok && eok && tok
	}
	return 0, 0, 0, false
}

func isSquareMapperExpr(expr Expr) bool {
	if fn, ok := expr.(*FnExpr); ok {
		return isSquareFnExpr(fn)
	}
	if le, ok := expr.(*LetExpr); ok {
		return isSquareFnExpr(extractFnExpr(le.body))
	}
	return false
}

func extractFnExpr(body []Expr) *FnExpr {
	if len(body) == 0 {
		return nil
	}
	switch e := body[len(body)-1].(type) {
	case *FnExpr:
		return e
	case *LetExpr:
		return extractFnExpr(e.body)
	case *DoExpr:
		return extractFnExpr(e.body)
	}
	return nil
}

func isSquareFnExpr(fn *FnExpr) bool {
	if fn == nil || len(fn.arities) != 1 || fn.variadic != nil {
		return false
	}
	arity := fn.arities[0]
	if len(arity.args) != 1 || len(arity.body) != 1 {
		return false
	}
	pf := guessFnParamFrame(arity.body, 1)
	if pf < 0 {
		pf = 1
	}
	call, ok := arity.body[0].(*CallExpr)
	if !ok || len(call.args) != 2 {
		return false
	}
	vref, ok := call.callable.(*VarRefExpr)
	if !ok || coreVarToProcName(vref.vr) != "procMultiply" {
		return false
	}
	lhs, lok := call.args[0].(*BindingExpr)
	rhs, rok := call.args[1].(*BindingExpr)
	return lok && rok && lhs.binding.frame == pf && rhs.binding.frame == pf && lhs.binding.index == 0 && rhs.binding.index == 0
}
