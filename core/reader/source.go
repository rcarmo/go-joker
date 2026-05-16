package reader

// FilenameOrDefault returns the reader source filename or the historical
// placeholder used when no filename is attached.
func FilenameOrDefault(f *string) string {
	if f != nil {
		return *f
	}
	return "<file>"
}
