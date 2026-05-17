package types

import "io"

type Equality interface {
	Equals(interface{}) bool
}

type Counted interface {
	Count() int
}

type Named interface {
	Name() string
	Namespace() string
}

type Printer interface {
	Print(writer io.Writer, printReadably bool)
}

type Pprinter interface {
	Pprint(writer io.Writer, indent int) int
}

type Formatter interface {
	Format(writer io.Writer, indent int) int
}

type Native interface {
	Native() interface{}
}

type Pending interface {
	IsRealized() bool
}
