package types

// StringSeq is a lazy seq over a string's runes; yields chars on demand.
// Runtime currently owns the descriptor carrier; root-generated bootstrap still
// installs the Type metadata until generated bootstrap ownership moves.
type StringSeq struct {
	S   string
	Off int
}
