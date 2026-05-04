package html

import (
	gohtml "html"
	"testing"
)

func BenchmarkNativeHTMLSmall(b *testing.B) {
	s := "Hello &amp; welcome to &lt;the&gt; &quot;world&quot;"
	for i := 0; i < b.N; i++ {
		gohtml.UnescapeString(s)
	}
}

func BenchmarkNativeHTMLMedium(b *testing.B) {
	s := "&lt;div class=&quot;container&quot;&gt;&lt;h1&gt;Title &amp; Subtitle&lt;/h1&gt;&lt;p&gt;This is &lt;em&gt;important&lt;/em&gt; &amp; &lt;strong&gt;bold&lt;/strong&gt; text.&lt;/p&gt;&lt;a href=&quot;https://example.com?a=1&amp;b=2&quot;&gt;Link&lt;/a&gt;&lt;/div&gt;"
	for i := 0; i < b.N; i++ {
		gohtml.UnescapeString(s)
	}
}
