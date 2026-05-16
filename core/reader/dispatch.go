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

// DispatchFormatPrefix returns the source prefix to preserve for dispatch
// macros whose format-mode representation is a prefix on the following form.
func DispatchFormatPrefix(kind DispatchKind) (string, bool) {
	switch kind {
	case DispatchVar:
		return "#'", true
	case DispatchDiscard:
		return "#_", true
	case DispatchMeta:
		return "#^", true
	case DispatchFn:
		return "#", true
	default:
		return "", false
	}
}

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
