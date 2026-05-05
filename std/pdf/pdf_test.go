package pdf

import (
	"os"
	"testing"

	. "github.com/candid82/joker/core"
)

func TestDocumentCreate(t *testing.T) {
	initPDFNamespace()

	doc := procDocument([]Object{MakeKeyword("a4")})
	if doc == nil {
		t.Fatal("document creation failed")
	}
	t.Logf("doc: %s", doc.ToString(false))

	// Add text (need a font first)
	// gopdf requires an explicit TTF font - skip text test without font file
	
	// Draw shapes (these work without fonts)
	procLine([]Object{doc, Double{D: 50}, Double{D: 50}, Double{D: 500}, Double{D: 50}})
	procRect([]Object{doc, Double{D: 50}, Double{D: 100}, Double{D: 200}, Double{D: 150}})
	procOval([]Object{doc, Double{D: 300}, Double{D: 200}, Double{D: 50}, Double{D: 30}})

	// Color
	procStrokeColor([]Object{doc, MakeInt(255), MakeInt(0), MakeInt(0)})
	procLine([]Object{doc, Double{D: 50}, Double{D: 300}, Double{D: 500}, Double{D: 300}})

	// New page
	procPage([]Object{doc})
	count := procPageCount([]Object{doc})
	if count.(Int).I != 2 {
		t.Fatalf("expected 2 pages, got %v", count)
	}

	// Save
	path := "/workspace/tmp/test-output.pdf"
	procSave([]Object{doc, MakeString(path)})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("PDF not written: %v", err)
	}
	t.Logf("PDF size: %d bytes", info.Size())
	os.Remove(path)
}
