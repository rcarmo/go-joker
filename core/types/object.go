package types

// Object is the root-independent read-only portion of the Joker object
// protocol. Root core still has a local Object alias while the remaining
// collection and runtime contracts are moved over incrementally.
type Object interface {
	Equality
	ToString(escape bool) string
	GetInfo() *ObjectInfo
	GetType() *Type
	Hash() uint32
}
