// Package reader is reserved for reader/parser extraction from root core.
//
// It contains root-independent reader mechanics such as character classes,
// comment/line/regex/string scanning, whitespace-skip decisions, dispatch/named character classification, symbolic-value lookup, character unicode/octal classification, delimiter/comment/terminating macro classification, delimiter token scanning, delimited form-count/arg-literal/conditional helpers, pending-form popping, arg-index gap filling, expected-token consumption, string escape classification, identifier rune checks/validation/explanations, identifier token scanning/validation, initial token classification, number/unicode token classification/scanning, string/unicode escape scanning/parsing, line rune readers,
// and raw IO wrappers. Reader and parser
// and expression construction call sites route through root core's
// ReaderConstructionAdapter, and the layout guard rejects future files in this
// package that import root core. Concrete reader/parser implementation should
// move here only after object construction, tagged-literal, namespace, error,
// and evaluator handoff dependencies are explicit and acyclic.
package reader
