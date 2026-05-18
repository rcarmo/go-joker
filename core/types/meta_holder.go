package types

type Meta interface {
	GetMeta() Map
	WithMeta(Map) Object
}

type MetaHolder struct {
	Meta Map
}

func (m MetaHolder) GetMeta() Map { return m.Meta }

func (m *MetaHolder) SetMeta(meta Map) { m.Meta = meta }
