package types

// RecordType describes a record class created by defrecord.
type RecordType struct {
	Name     string
	Fields   []string
	FieldIdx map[string]int
}

func MakeRecordType(name string, fields []string) *RecordType {
	idx := make(map[string]int)
	for i, f := range fields {
		idx[f] = i
	}
	return &RecordType{Name: name, Fields: fields, FieldIdx: idx}
}
