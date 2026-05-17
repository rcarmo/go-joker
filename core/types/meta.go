package types

type MetadataFactory func(doc string, name string) any

func TypeMetadataDoc(kind Kind, doc string) string {
	if doc != "" {
		doc = "\n  " + doc
	}
	return kind.DocumentationPrefix() + doc
}
