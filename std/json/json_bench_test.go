package json

import "testing"

func BenchmarkNativeJSONSmall(b *testing.B) {
	s := `{"name":"John","age":30,"city":"New York","active":true,"scores":[95,87,92]}`
	for i := 0; i < b.N; i++ {
		readString(s, nil)
	}
}

func BenchmarkNativeJSONMedium(b *testing.B) {
	s := `[{"id":1,"name":"Alice","email":"alice@test.com","tags":["admin","user"],"score":95},{"id":2,"name":"Bob","email":"bob@test.com","tags":["user"],"score":87},{"id":3,"name":"Charlie","email":"charlie@test.com","tags":["user","mod"],"score":92},{"id":4,"name":"Dave","email":"dave@test.com","tags":[],"score":78},{"id":5,"name":"Eve","email":"eve@test.com","tags":["admin"],"score":99}]`
	for i := 0; i < b.N; i++ {
		readString(s, nil)
	}
}
