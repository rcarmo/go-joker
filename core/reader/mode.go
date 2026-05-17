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
	EDNDialect Dialect = iota
	CLJDialect
	CLJSDialect
	CLJXDialect
)
