package yaml

import "testing"

func BenchmarkNativeYAMLSmall(b *testing.B) {
	s := "name: John\nage: 30\ncity: New York\nactive: true\n"
	for i := 0; i < b.N; i++ {
		readString(s)
	}
}

func BenchmarkNativeYAMLMedium(b *testing.B) {
	s := "id: 1\nname: Alice\nemail: alice@test.com\nscore: 95\nrole: admin\nverified: true\nid2: 2\nname2: Bob\nemail2: bob@test.com\nscore2: 87\nrole2: user\nverified2: false\n"
	for i := 0; i < b.N; i++ {
		readString(s)
	}
}
