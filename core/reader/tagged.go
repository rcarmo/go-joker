package reader

// DefaultDataReaderFnVarName returns the root var name consulted for fallback
// tagged literal handling.
func DefaultDataReaderFnVarName() string {
	return "*default-data-reader-fn*"
}

// DataReaderVarNames returns the root var names consulted for tagged literal
// readers, in lookup order. Root owns namespace access and concrete Map lookup.
func DataReaderVarNames() []string {
	return []string{"*data-readers*", "default-data-readers"}
}

// MissingTaggedReaderAction describes how root should handle a tagged literal
// without a registered reader. Root owns concrete warning/error construction.
type MissingTaggedReaderAction int

const (
	MissingTaggedReaderReturnValue MissingTaggedReaderAction = iota
	MissingTaggedReaderWarnAndReturnValue
	MissingTaggedReaderError
)

// ClassifyMissingTaggedReaderAction returns the action for missing tagged
// literal readers under current root reader modes.
func ClassifyMissingTaggedReaderAction(suppressRead bool, linterMode bool, ednDialect bool) MissingTaggedReaderAction {
	switch {
	case suppressRead:
		return MissingTaggedReaderReturnValue
	case linterMode && !ednDialect:
		return MissingTaggedReaderWarnAndReturnValue
	case linterMode:
		return MissingTaggedReaderReturnValue
	default:
		return MissingTaggedReaderError
	}
}

// TaggedLiteralPrefix returns the format-mode source prefix for a tagged
// literal after the tag token has been read. Root owns concrete tag Object
// validation and string conversion.
func TaggedLiteralPrefix(tag string) string {
	return "#" + tag + " "
}
