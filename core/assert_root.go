package core

import coretypes "github.com/rcarmo/go-joker/core/types"

func EnsureObjectIsNamespace(obj coretypes.Object, pattern string) *Namespace {
	if c, yes := obj.(*Namespace); yes {
		return c
	}
	panic(FailObject(obj, "Namespace", pattern))
}

func EnsureArgIsNamespace(args []coretypes.Object, index int) *Namespace {
	obj := args[index]
	if c, yes := obj.(*Namespace); yes {
		return c
	}
	panic(FailArg(obj, "Namespace", index))
}

func EnsureObjectIsVar(obj coretypes.Object, pattern string) *Var {
	if c, yes := obj.(*Var); yes {
		return c
	}
	panic(FailObject(obj, "Var", pattern))
}

func EnsureArgIsVar(args []coretypes.Object, index int) *Var {
	obj := args[index]
	if c, yes := obj.(*Var); yes {
		return c
	}
	panic(FailArg(obj, "Var", index))
}

func EnsureObjectIsFn(obj coretypes.Object, pattern string) *Fn {
	if c, yes := obj.(*Fn); yes {
		return c
	}
	panic(FailObject(obj, "Fn", pattern))
}

func EnsureArgIsFn(args []coretypes.Object, index int) *Fn {
	obj := args[index]
	if c, yes := obj.(*Fn); yes {
		return c
	}
	panic(FailArg(obj, "Fn", index))
}

func EnsureObjectIsAtom(obj coretypes.Object, pattern string) *Atom {
	if c, yes := obj.(*Atom); yes {
		return c
	}
	panic(FailObject(obj, "Atom", pattern))
}

func EnsureArgIsAtom(args []coretypes.Object, index int) *Atom {
	obj := args[index]
	if c, yes := obj.(*Atom); yes {
		return c
	}
	panic(FailArg(obj, "Atom", index))
}

func EnsureObjectIsFile(obj coretypes.Object, pattern string) *File {
	if c, yes := obj.(*File); yes {
		return c
	}
	panic(FailObject(obj, "File", pattern))
}

func EnsureArgIsFile(args []coretypes.Object, index int) *File {
	obj := args[index]
	if c, yes := obj.(*File); yes {
		return c
	}
	panic(FailArg(obj, "File", index))
}

func EnsureObjectIsChannel(obj coretypes.Object, pattern string) *Channel {
	if c, yes := obj.(*Channel); yes {
		return c
	}
	panic(FailObject(obj, "Channel", pattern))
}

func EnsureArgIsChannel(args []coretypes.Object, index int) *Channel {
	obj := args[index]
	if c, yes := obj.(*Channel); yes {
		return c
	}
	panic(FailArg(obj, "Channel", index))
}
