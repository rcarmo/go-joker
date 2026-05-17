package types

// Object is the root-independent read-only portion of the Joker object
// protocol. Root core extends it with metadata mutation and concrete Type
// ownership until those groups move as well.
type Object interface {
	Equality
	ToString(escape bool) string
	GetInfo() *ObjectInfo
	Hash() uint32
}
