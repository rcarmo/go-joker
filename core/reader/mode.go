package reader

type Phase int

const (
	ReadPhase Phase = iota
	FormatPhase
	ParsePhase
	EvalPhase
	PrintIfNotNilPhase
)

type Dialect int

const (
	CLJDialect Dialect = iota
	CLJSDialect
	JokerDialect
	EDNDialect
	UnknownDialect
)
