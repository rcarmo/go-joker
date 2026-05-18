package types

import (
	"fmt"
	"io"
)

var AssertionFailObject func(Object, string, string) any
var AssertionFailArg func(Object, string, int) any

func failObject(obj Object, typeName, pattern string) {
	if AssertionFailObject != nil {
		panic(AssertionFailObject(obj, typeName, pattern))
	}
	if pattern == "" {
		pattern = "%s"
	}
	panic(fmt.Sprintf(pattern, fmt.Sprintf("Expected %s, got %s", typeName, obj.GetType().ToString(false))))
}

func failArg(obj Object, typeName string, index int) {
	if AssertionFailArg != nil {
		panic(AssertionFailArg(obj, typeName, index))
	}
	panic(fmt.Sprintf("Arg %d expected %s, got %s", index, typeName, obj.GetType().ToString(false)))
}

func EnsureObjectIsComparable(obj Object, pattern string) Comparable {
	if c, yes := obj.(Comparable); yes {
		return c
	}
	failObject(obj, "Comparable", pattern)
	var zero Comparable
	return zero
}

func EnsureArgIsComparable(args []Object, index int) Comparable {
	obj := args[index]
	if c, yes := obj.(Comparable); yes {
		return c
	}
	failArg(obj, "Comparable", index)
	var zero Comparable
	return zero
}

func EnsureObjectIsChar(obj Object, pattern string) Char {
	if c, yes := obj.(Char); yes {
		return c
	}
	failObject(obj, "Char", pattern)
	var zero Char
	return zero
}

func EnsureArgIsChar(args []Object, index int) Char {
	obj := args[index]
	if c, yes := obj.(Char); yes {
		return c
	}
	failArg(obj, "Char", index)
	var zero Char
	return zero
}

func EnsureObjectIsString(obj Object, pattern string) String {
	if c, yes := obj.(String); yes {
		return c
	}
	failObject(obj, "String", pattern)
	var zero String
	return zero
}

func EnsureArgIsString(args []Object, index int) String {
	obj := args[index]
	if c, yes := obj.(String); yes {
		return c
	}
	failArg(obj, "String", index)
	var zero String
	return zero
}

func EnsureObjectIsStringable(obj Object, pattern string) String {
	switch c := obj.(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		failObject(c, "Stringable", pattern)
		var zero String
		return zero
	}
}

func EnsureArgIsStringable(args []Object, index int) String {
	switch c := args[index].(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		failArg(c, "Stringable", index)
		var zero String
		return zero
	}
}

func EnsureObjectIsSymbol(obj Object, pattern string) Symbol {
	if c, yes := obj.(Symbol); yes {
		return c
	}
	failObject(obj, "Symbol", pattern)
	var zero Symbol
	return zero
}

func EnsureArgIsSymbol(args []Object, index int) Symbol {
	obj := args[index]
	if c, yes := obj.(Symbol); yes {
		return c
	}
	failArg(obj, "Symbol", index)
	var zero Symbol
	return zero
}

func EnsureObjectIsKeyword(obj Object, pattern string) Keyword {
	if c, yes := obj.(Keyword); yes {
		return c
	}
	failObject(obj, "Keyword", pattern)
	var zero Keyword
	return zero
}

func EnsureArgIsKeyword(args []Object, index int) Keyword {
	obj := args[index]
	if c, yes := obj.(Keyword); yes {
		return c
	}
	failArg(obj, "Keyword", index)
	var zero Keyword
	return zero
}

func EnsureObjectIsRegex(obj Object, pattern string) *Regex {
	if c, yes := obj.(*Regex); yes {
		return c
	}
	failObject(obj, "Regex", pattern)
	var zero *Regex
	return zero
}

func EnsureArgIsRegex(args []Object, index int) *Regex {
	obj := args[index]
	if c, yes := obj.(*Regex); yes {
		return c
	}
	failArg(obj, "Regex", index)
	var zero *Regex
	return zero
}

func EnsureObjectIsBoolean(obj Object, pattern string) Boolean {
	if c, yes := obj.(Boolean); yes {
		return c
	}
	failObject(obj, "Boolean", pattern)
	var zero Boolean
	return zero
}

func EnsureArgIsBoolean(args []Object, index int) Boolean {
	obj := args[index]
	if c, yes := obj.(Boolean); yes {
		return c
	}
	failArg(obj, "Boolean", index)
	var zero Boolean
	return zero
}

func EnsureObjectIsTime(obj Object, pattern string) Time {
	if c, yes := obj.(Time); yes {
		return c
	}
	failObject(obj, "Time", pattern)
	var zero Time
	return zero
}

func EnsureArgIsTime(args []Object, index int) Time {
	obj := args[index]
	if c, yes := obj.(Time); yes {
		return c
	}
	failArg(obj, "Time", index)
	var zero Time
	return zero
}

func EnsureObjectIsNumber(obj Object, pattern string) Number {
	if c, yes := obj.(Number); yes {
		return c
	}
	failObject(obj, "Number", pattern)
	var zero Number
	return zero
}

func EnsureArgIsNumber(args []Object, index int) Number {
	obj := args[index]
	if c, yes := obj.(Number); yes {
		return c
	}
	failArg(obj, "Number", index)
	var zero Number
	return zero
}

func EnsureObjectIsSeqable(obj Object, pattern string) Seqable {
	if c, yes := obj.(Seqable); yes {
		return c
	}
	failObject(obj, "Seqable", pattern)
	var zero Seqable
	return zero
}

func EnsureArgIsSeqable(args []Object, index int) Seqable {
	obj := args[index]
	if c, yes := obj.(Seqable); yes {
		return c
	}
	failArg(obj, "Seqable", index)
	var zero Seqable
	return zero
}

func EnsureObjectIsCallable(obj Object, pattern string) Callable {
	if c, yes := obj.(Callable); yes {
		return c
	}
	failObject(obj, "Callable", pattern)
	var zero Callable
	return zero
}

func EnsureArgIsCallable(args []Object, index int) Callable {
	obj := args[index]
	if c, yes := obj.(Callable); yes {
		return c
	}
	failArg(obj, "Callable", index)
	var zero Callable
	return zero
}

func EnsureObjectIsType(obj Object, pattern string) *Type {
	if c, yes := obj.(*Type); yes {
		return c
	}
	failObject(obj, "Type", pattern)
	var zero *Type
	return zero
}

func EnsureArgIsType(args []Object, index int) *Type {
	obj := args[index]
	if c, yes := obj.(*Type); yes {
		return c
	}
	failArg(obj, "Type", index)
	var zero *Type
	return zero
}

func EnsureObjectIsInt(obj Object, pattern string) Int {
	if c, yes := obj.(Int); yes {
		return c
	}
	failObject(obj, "Int", pattern)
	var zero Int
	return zero
}

func EnsureArgIsInt(args []Object, index int) Int {
	obj := args[index]
	if c, yes := obj.(Int); yes {
		return c
	}
	failArg(obj, "Int", index)
	var zero Int
	return zero
}

func EnsureObjectIsDouble(obj Object, pattern string) Double {
	if c, yes := obj.(Double); yes {
		return c
	}
	failObject(obj, "Double", pattern)
	var zero Double
	return zero
}

func EnsureArgIsDouble(args []Object, index int) Double {
	obj := args[index]
	if c, yes := obj.(Double); yes {
		return c
	}
	failArg(obj, "Double", index)
	var zero Double
	return zero
}

func EnsureObjectIsStack(obj Object, pattern string) Stack {
	if c, yes := obj.(Stack); yes {
		return c
	}
	failObject(obj, "Stack", pattern)
	var zero Stack
	return zero
}

func EnsureArgIsStack(args []Object, index int) Stack {
	obj := args[index]
	if c, yes := obj.(Stack); yes {
		return c
	}
	failArg(obj, "Stack", index)
	var zero Stack
	return zero
}

func EnsureObjectIsAssociative(obj Object, pattern string) Associative {
	if c, yes := obj.(Associative); yes {
		return c
	}
	failObject(obj, "Associative", pattern)
	var zero Associative
	return zero
}

func EnsureArgIsAssociative(args []Object, index int) Associative {
	obj := args[index]
	if c, yes := obj.(Associative); yes {
		return c
	}
	failArg(obj, "Associative", index)
	var zero Associative
	return zero
}

func EnsureObjectIsReversible(obj Object, pattern string) Reversible {
	if c, yes := obj.(Reversible); yes {
		return c
	}
	failObject(obj, "Reversible", pattern)
	var zero Reversible
	return zero
}

func EnsureArgIsReversible(args []Object, index int) Reversible {
	obj := args[index]
	if c, yes := obj.(Reversible); yes {
		return c
	}
	failArg(obj, "Reversible", index)
	var zero Reversible
	return zero
}

func EnsureObjectIsNamed(obj Object, pattern string) Named {
	if c, yes := obj.(Named); yes {
		return c
	}
	failObject(obj, "Named", pattern)
	var zero Named
	return zero
}

func EnsureArgIsNamed(args []Object, index int) Named {
	obj := args[index]
	if c, yes := obj.(Named); yes {
		return c
	}
	failArg(obj, "Named", index)
	var zero Named
	return zero
}

func EnsureObjectIsComparator(obj Object, pattern string) Comparator {
	if c, yes := obj.(Comparator); yes {
		return c
	}
	failObject(obj, "Comparator", pattern)
	var zero Comparator
	return zero
}

func EnsureArgIsComparator(args []Object, index int) Comparator {
	obj := args[index]
	if c, yes := obj.(Comparator); yes {
		return c
	}
	failArg(obj, "Comparator", index)
	var zero Comparator
	return zero
}

func EnsureObjectIsRatio(obj Object, pattern string) *Ratio {
	if c, yes := obj.(*Ratio); yes {
		return c
	}
	failObject(obj, "Ratio", pattern)
	var zero *Ratio
	return zero
}

func EnsureArgIsRatio(args []Object, index int) *Ratio {
	obj := args[index]
	if c, yes := obj.(*Ratio); yes {
		return c
	}
	failArg(obj, "Ratio", index)
	var zero *Ratio
	return zero
}

func EnsureObjectIsBigFloat(obj Object, pattern string) *BigFloat {
	if c, yes := obj.(*BigFloat); yes {
		return c
	}
	failObject(obj, "BigFloat", pattern)
	var zero *BigFloat
	return zero
}

func EnsureArgIsBigFloat(args []Object, index int) *BigFloat {
	obj := args[index]
	if c, yes := obj.(*BigFloat); yes {
		return c
	}
	failArg(obj, "BigFloat", index)
	var zero *BigFloat
	return zero
}

func EnsureObjectIsBigInt(obj Object, pattern string) *BigInt {
	if c, yes := obj.(*BigInt); yes {
		return c
	}
	failObject(obj, "BigInt", pattern)
	var zero *BigInt
	return zero
}

func EnsureArgIsBigInt(args []Object, index int) *BigInt {
	obj := args[index]
	if c, yes := obj.(*BigInt); yes {
		return c
	}
	failArg(obj, "BigInt", index)
	var zero *BigInt
	return zero
}

func EnsureObjectIsError(obj Object, pattern string) Error {
	if c, yes := obj.(Error); yes {
		return c
	}
	failObject(obj, "Error", pattern)
	var zero Error
	return zero
}

func EnsureArgIsError(args []Object, index int) Error {
	obj := args[index]
	if c, yes := obj.(Error); yes {
		return c
	}
	failArg(obj, "Error", index)
	var zero Error
	return zero
}

func EnsureObjectIsDeref(obj Object, pattern string) Deref {
	if c, yes := obj.(Deref); yes {
		return c
	}
	failObject(obj, "Deref", pattern)
	var zero Deref
	return zero
}

func EnsureArgIsDeref(args []Object, index int) Deref {
	obj := args[index]
	if c, yes := obj.(Deref); yes {
		return c
	}
	failArg(obj, "Deref", index)
	var zero Deref
	return zero
}

func EnsureObjectIsKVReduce(obj Object, pattern string) KVReduce {
	if c, yes := obj.(KVReduce); yes {
		return c
	}
	failObject(obj, "KVReduce", pattern)
	var zero KVReduce
	return zero
}

func EnsureArgIsKVReduce(args []Object, index int) KVReduce {
	obj := args[index]
	if c, yes := obj.(KVReduce); yes {
		return c
	}
	failArg(obj, "KVReduce", index)
	var zero KVReduce
	return zero
}

func EnsureObjectIsReduce(obj Object, pattern string) Reduce {
	if c, yes := obj.(Reduce); yes {
		return c
	}
	failObject(obj, "Reduce", pattern)
	var zero Reduce
	return zero
}

func EnsureArgIsReduce(args []Object, index int) Reduce {
	obj := args[index]
	if c, yes := obj.(Reduce); yes {
		return c
	}
	failArg(obj, "Reduce", index)
	var zero Reduce
	return zero
}

func EnsureObjectIsPending(obj Object, pattern string) Pending {
	if c, yes := obj.(Pending); yes {
		return c
	}
	failObject(obj, "Pending", pattern)
	var zero Pending
	return zero
}

func EnsureArgIsPending(args []Object, index int) Pending {
	obj := args[index]
	if c, yes := obj.(Pending); yes {
		return c
	}
	failArg(obj, "Pending", index)
	var zero Pending
	return zero
}

func EnsureObjectIsStringReader(obj Object, pattern string) StringReader {
	if c, yes := obj.(StringReader); yes {
		return c
	}
	failObject(obj, "StringReader", pattern)
	var zero StringReader
	return zero
}

func EnsureArgIsStringReader(args []Object, index int) StringReader {
	obj := args[index]
	if c, yes := obj.(StringReader); yes {
		return c
	}
	failArg(obj, "StringReader", index)
	var zero StringReader
	return zero
}

func EnsureObjectIsCountedIndexed(obj Object, pattern string) CountedIndexed {
	if c, yes := obj.(CountedIndexed); yes {
		return c
	}
	failObject(obj, "CountedIndexed", pattern)
	var zero CountedIndexed
	return zero
}

func EnsureArgIsCountedIndexed(args []Object, index int) CountedIndexed {
	obj := args[index]
	if c, yes := obj.(CountedIndexed); yes {
		return c
	}
	failArg(obj, "CountedIndexed", index)
	var zero CountedIndexed
	return zero
}

func EnsureObjectIsMeta(obj Object, pattern string) Meta {
	if c, yes := obj.(Meta); yes {
		return c
	}
	failObject(obj, "Meta", pattern)
	var zero Meta
	return zero
}

func EnsureArgIsMeta(args []Object, index int) Meta {
	obj := args[index]
	if c, yes := obj.(Meta); yes {
		return c
	}
	failArg(obj, "Meta", index)
	var zero Meta
	return zero
}

func EnsureObjectIsMap(obj Object, pattern string) Map {
	if c, yes := obj.(Map); yes {
		return c
	}
	failObject(obj, "Map", pattern)
	var zero Map
	return zero
}

func EnsureArgIsMap(args []Object, index int) Map {
	obj := args[index]
	if c, yes := obj.(Map); yes {
		return c
	}
	failArg(obj, "Map", index)
	var zero Map
	return zero
}

func EnsureObjectIsSet(obj Object, pattern string) Set {
	if c, yes := obj.(Set); yes {
		return c
	}
	failObject(obj, "Set", pattern)
	var zero Set
	return zero
}

func EnsureArgIsSet(args []Object, index int) Set {
	obj := args[index]
	if c, yes := obj.(Set); yes {
		return c
	}
	failArg(obj, "Set", index)
	var zero Set
	return zero
}

func EnsureObjectIsVec(obj Object, pattern string) Vec {
	if c, yes := obj.(Vec); yes {
		return c
	}
	failObject(obj, "Vec", pattern)
	var zero Vec
	return zero
}

func EnsureArgIsVec(args []Object, index int) Vec {
	obj := args[index]
	if c, yes := obj.(Vec); yes {
		return c
	}
	failArg(obj, "Vec", index)
	var zero Vec
	return zero
}

func EnsureObjectIsio_Reader(obj Object, pattern string) io.Reader {
	if c, yes := obj.(io.Reader); yes {
		return c
	}
	failObject(obj, "io.Reader", pattern)
	var zero io.Reader
	return zero
}

func EnsureArgIsio_Reader(args []Object, index int) io.Reader {
	obj := args[index]
	if c, yes := obj.(io.Reader); yes {
		return c
	}
	failArg(obj, "io.Reader", index)
	var zero io.Reader
	return zero
}

func EnsureObjectIsio_Writer(obj Object, pattern string) io.Writer {
	if c, yes := obj.(io.Writer); yes {
		return c
	}
	failObject(obj, "io.Writer", pattern)
	var zero io.Writer
	return zero
}

func EnsureArgIsio_Writer(args []Object, index int) io.Writer {
	obj := args[index]
	if c, yes := obj.(io.Writer); yes {
		return c
	}
	failArg(obj, "io.Writer", index)
	var zero io.Writer
	return zero
}

func EnsureObjectIsio_RuneReader(obj Object, pattern string) io.RuneReader {
	if c, yes := obj.(io.RuneReader); yes {
		return c
	}
	failObject(obj, "io.RuneReader", pattern)
	var zero io.RuneReader
	return zero
}

func EnsureArgIsio_RuneReader(args []Object, index int) io.RuneReader {
	obj := args[index]
	if c, yes := obj.(io.RuneReader); yes {
		return c
	}
	failArg(obj, "io.RuneReader", index)
	var zero io.RuneReader
	return zero
}

func EnsureObjectIsRef(obj Object, pattern string) Ref {
	if c, yes := obj.(Ref); yes {
		return c
	}
	failObject(obj, "Ref", pattern)
	var zero Ref
	return zero
}

func EnsureArgIsRef(args []Object, index int) Ref {
	obj := args[index]
	if c, yes := obj.(Ref); yes {
		return c
	}
	failArg(obj, "Ref", index)
	var zero Ref
	return zero
}
