package reader

// TaggedLiteralPrefix returns the format-mode source prefix for a tagged
// literal after the tag token has been read. Root owns concrete tag Object
// validation and string conversion.
func TaggedLiteralPrefix(tag string) string {
	return "#" + tag + " "
}
