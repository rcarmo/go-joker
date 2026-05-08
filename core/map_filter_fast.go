package core

// map_filter_fast.go — AST-level fused reducible pipelines.
//
// Recognizes reduce over a map/filter/take pipeline rooted at range and executes
// it in one loop, avoiding lazy seq wrapper churn. This replaces the earlier
// square/even/take/range-only special case with a small general pipeline compiler.

type reducibleStepKind byte

const (
	reducibleMap reducibleStepKind = iota
	reducibleFilter
	reducibleTake
)

type reducibleIntrinsic byte

const (
	reducibleGeneric reducibleIntrinsic = iota
	reducibleSquareInt
	reducibleEvenInt
)

type reducibleStep struct {
	kind      reducibleStepKind
	intrinsic reducibleIntrinsic
	fn        Callable
	takeLimit int
}

type reducibleRangePipeline struct {
	start int
	end   int
	step  int
	steps []reducibleStep
}

func evalReducePipelineFast(expr *CallExpr, env *LocalEnv) (Object, bool) {
	if !callableName(expr.callable, "reduce") || (len(expr.args) != 2 && len(expr.args) != 3) {
		return nil, false
	}

	reducerObj := Eval(expr.args[0], env)
	reducer, ok := reducerObj.(Callable)
	if !ok {
		return nil, false
	}

	var init Object
	var collExpr Expr
	if len(expr.args) == 3 {
		init = Eval(expr.args[1], env)
		collExpr = expr.args[2]
	} else {
		collExpr = expr.args[1]
	}

	pipeline, ok := compileReducibleRangePipeline(collExpr, env)
	if !ok || pipeline.step == 0 || len(pipeline.steps) == 0 {
		return nil, false
	}

	if len(expr.args) == 2 {
		return reducePipelineNoInit(reducer, pipeline)
	}
	return reducePipelineInit(reducer, init, pipeline), true
}

func compileReducibleRangePipeline(expr Expr, env *LocalEnv) (reducibleRangePipeline, bool) {
	call, ok := expr.(*CallExpr)
	if !ok {
		return reducibleRangePipeline{}, false
	}

	if callableName(call.callable, "range") {
		start, end, step, ok := evalRangeArgs(call.args, env)
		if !ok || step == 0 {
			return reducibleRangePipeline{}, false
		}
		return reducibleRangePipeline{start: start, end: end, step: step}, true
	}

	if callableName(call.callable, "map") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if isSquareMapperExpr(call.args[0]) {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, intrinsic: reducibleSquareInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "filter") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if callableName(call.args[0], "even?") {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, intrinsic: reducibleEvenInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "take") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		nObj := Eval(call.args[0], env)
		n, ok := nObj.(Int)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleTake, takeLimit: n.I})
		return inner, true
	}

	return reducibleRangePipeline{}, false
}

func reducePipelineNoInit(reducer Callable, p reducibleRangePipeline) (Object, bool) {
	seen := false
	var acc Object
	_, stopped := walkReducibleRangePipeline(p, func(v Object) bool {
		if !seen {
			acc = v
			seen = true
			return false
		}
		acc = reduceStepFast(reducer, acc, v)
		return IsReduced(acc)
	})
	if !seen {
		return call0(reducer), true
	}
	if stopped && IsReduced(acc) {
		return DerefReduced(acc), true
	}
	return acc, true
}

func reducePipelineInit(reducer Callable, init Object, p reducibleRangePipeline) Object {
	acc := init
	reducerName := hotReducerName(reducer)
	_, stopped := walkReducibleRangePipeline(p, func(v Object) bool {
		acc = reduceStepFastByName(reducer, reducerName, acc, v)
		return IsReduced(acc)
	})
	if stopped && IsReduced(acc) {
		return DerefReduced(acc)
	}
	return acc
}

func walkReducibleRangePipeline(p reducibleRangePipeline, emit func(Object) bool) (emitted int, stopped bool) {
	takeRemaining := make([]int, len(p.steps))
	for i, step := range p.steps {
		if step.kind == reducibleTake {
			takeRemaining[i] = step.takeLimit
		}
	}

	for i := p.start; (p.step > 0 && i < p.end) || (p.step < 0 && i > p.end); i += p.step {
		v := Object(Int{I: i})
		alive := true
		stopAfterCurrent := false

		for si, step := range p.steps {
			if !alive {
				break
			}
			switch step.kind {
			case reducibleMap:
				if step.intrinsic == reducibleSquareInt {
					if iv, ok := v.(Int); ok {
						v = Int{I: iv.I * iv.I}
					} else {
						v = call1(step.fn, v)
					}
				} else {
					v = call1(step.fn, v)
				}
			case reducibleFilter:
				if step.intrinsic == reducibleEvenInt {
					if iv, ok := v.(Int); ok {
						alive = iv.I%2 == 0
					} else {
						alive = false
					}
				} else if !ToBool(call1(step.fn, v)) {
					alive = false
				}
			case reducibleTake:
				if takeRemaining[si] <= 0 {
					return emitted, true
				}
				takeRemaining[si]--
				if takeRemaining[si] == 0 {
					stopAfterCurrent = true
				}
			}
		}

		if alive {
			emitted++
			if emit(v) {
				return emitted, true
			}
		}
		if stopAfterCurrent {
			return emitted, true
		}
	}
	return emitted, false
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
