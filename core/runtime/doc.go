// Package runtime is reserved for root-runtime extraction.
//
// It intentionally contains no production runtime yet. Go packages are
// directory-scoped, so moving evaluator/executor files here must happen only
// after their object, call, error, and frame dependencies are expressed as
// explicit contracts instead of unexported core package reach-through.
package runtime
