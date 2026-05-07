package core

// call_fast.go — stack-allocated helper calls for hot Callable paths.
//
// Avoids repeated []Object literal allocation at call sites such as reduce,
// transducers, watches, and comparators.

func call0(c Callable) Object {
	return c.Call(nil)
}

func call1(c Callable, a Object) Object {
	var args [1]Object
	args[0] = a
	return c.Call(args[:])
}

func call2(c Callable, a, b Object) Object {
	var args [2]Object
	args[0] = a
	args[1] = b
	return c.Call(args[:])
}

func call3(c Callable, a, b, d Object) Object {
	var args [3]Object
	args[0] = a
	args[1] = b
	args[2] = d
	return c.Call(args[:])
}

func call4(c Callable, a, b, d, e Object) Object {
	var args [4]Object
	args[0] = a
	args[1] = b
	args[2] = d
	args[3] = e
	return c.Call(args[:])
}
