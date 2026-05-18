package types

func WithInfo(obj Object, info *ObjectInfo) Object {
	if h, ok := obj.(interface {
		WithInfo(*ObjectInfo) Object
	}); ok {
		return h.WithInfo(info)
	}
	return obj
}

func RootObject(obj Object) Object {
	return obj.(Object)
}
