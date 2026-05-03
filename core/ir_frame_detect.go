package core

// ir_frame_detect.go — precise frame detection for let/loop bindings.
//
// The IR compiler needs to know which parse-time frame each let/loop
// binding belongs to. Instead of guessing via heuristics, this scans
// the body for binding references and deduces the frame from the
// binding indices.

// findLetFrame determines the parse-time frame for a let expression's
// bindings. It scans the body for BindingExpr nodes with indices 0..nBinds-1
// that reference a frame not already known in the compiler's bindingMap.
func findLetFrame(body []Expr, nBinds int, known map[bindingKey]int) int {
	if nBinds == 0 {
		return -1
	}
	// Collect candidate frames: frames where index 0 appears and is NOT in known
	candidates := map[int]int{} // frame -> count of matching indices
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *BindingExpr:
			f, idx := x.binding.frame, x.binding.index
			if idx < nBinds {
				if _, alreadyKnown := known[bindingKey{frame: f, index: idx}]; !alreadyKnown {
					candidates[f]++
				}
			}
		case *IfExpr:
			scan(x.cond)
			scan(x.positive)
			scan(x.negative)
		case *CallExpr:
			scan(x.callable)
			for _, a := range x.args {
				scan(a)
			}
		case *RecurExpr:
			for _, a := range x.args {
				scan(a)
			}
		case *LetExpr:
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *LoopExpr:
			le := (*LetExpr)(x)
			for _, v := range le.values {
				scan(v)
			}
			for _, b := range le.body {
				scan(b)
			}
		}
	}
	for _, e := range body {
		scan(e)
	}

	// Pick the candidate frame where count matches nBinds exactly
	// (the let's own frame should have exactly nBinds distinct indices)
	bestFrame := -1
	for f, count := range candidates {
		if count == nBinds {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	// Fallback: pick the smallest frame with any matches
	if bestFrame < 0 {
		for f := range candidates {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	return bestFrame
}
