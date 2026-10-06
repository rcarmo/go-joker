package math

import core "github.com/rcarmo/go-joker/v42/core"

// math_extra has already loaded the generated namespace before user code runs.
func init() {
	for _, name := range []string{"sqrt", "floor", "abs"} {
		core.RegisterNumericPrimitive(mathNamespace.Resolve(name))
	}
}
