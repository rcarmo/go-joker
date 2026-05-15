package reader

type DispatchKind int

const (
	DispatchTagged DispatchKind = iota
	DispatchRegex
	DispatchVar
	DispatchDiscard
	DispatchMeta
	DispatchSet
	DispatchFn
	DispatchConditional
	DispatchNamespacedMap
	DispatchSymbolicValue
)

func ClassifyDispatch(r rune) DispatchKind {
	switch r {
	case '"':
		return DispatchRegex
	case '\'':
		return DispatchVar
	case '_':
		return DispatchDiscard
	case '^':
		return DispatchMeta
	case '{':
		return DispatchSet
	case '(':
		return DispatchFn
	case '?':
		return DispatchConditional
	case ':':
		return DispatchNamespacedMap
	case '#':
		return DispatchSymbolicValue
	default:
		return DispatchTagged
	}
}
