package types

func NumberEqualsDefault(x Number, y interface{}) bool {
	switch y := y.(type) {
	case Number:
		return Category(x) == Category(y) && NumbersEq(x, y)
	default:
		return false
	}
}
