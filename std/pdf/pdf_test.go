package pdf

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

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
	path := filepath.Join(t.TempDir(), "test-output.pdf")
	procSave([]Object{doc, MakeString(path)})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("PDF not written: %v", err)
	}
	t.Logf("PDF size: %d bytes", info.Size())
	os.Remove(path)
}

func TestPDFColorRejectsOutOfRangeChannels(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procColor([]Object{doc, MakeInt(256), MakeInt(0), MakeInt(0)})
	})
	expectPanic(t, func() {
		procStrokeColor([]Object{doc, MakeInt(0), MakeInt(-1), MakeInt(0)})
	})
	expectPanic(t, func() {
		procFillColor([]Object{doc, MakeInt(0), MakeInt(0), MakeInt(999)})
	})
}

func TestPDFGeometryRejectsNonFiniteDimensions(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procRect([]Object{doc, Double{D: 10}, Double{D: 10}, Double{D: math.Inf(1)}, Double{D: 10}})
	})
	expectPanic(t, func() {
		procMargins([]Object{doc, Double{D: 10}, Double{D: math.NaN()}, Double{D: 10}, Double{D: 10}})
	})
}

func TestPDFGeometryRejectsInvalidDimensions(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procTextWrap([]Object{doc, Double{D: 10}, Double{D: 10}, Double{D: 0}, MakeString("x")})
	})
	expectPanic(t, func() {
		procRect([]Object{doc, Double{D: 10}, Double{D: 10}, Double{D: -1}, Double{D: 10}})
	})
	expectPanic(t, func() {
		procOval([]Object{doc, Double{D: 10}, Double{D: 10}, Double{D: 1}, Double{D: 0}})
	})
	expectPanic(t, func() {
		procImage([]Object{doc, MakeString(filepath.Join(t.TempDir(), "missing.png")), Double{D: 10}, Double{D: 10}, Double{D: -1}})
	})
	expectPanic(t, func() {
		procLink([]Object{doc, MakeString("https://example.com"), Double{D: 10}, Double{D: 10}, Double{D: 0}, Double{D: 10}})
	})
	expectPanic(t, func() {
		procMargins([]Object{doc, Double{D: 10}, Double{D: -1}, Double{D: 10}, Double{D: 10}})
	})
}

func TestPDFLineWidthRejectsInvalidValue(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procLineWidth([]Object{doc, Double{D: 0}})
	})
}

func TestPDFDocumentRejectsInvalidDimensions(t *testing.T) {
	initPDFNamespace()
	expectPanic(t, func() {
		procDocument([]Object{Double{D: 0}, Double{D: 100}})
	})
	expectPanic(t, func() {
		procDocument([]Object{Double{D: 100}, Double{D: -1}})
	})
	expectPanic(t, func() {
		procDocument([]Object{MakeKeyword("bogus")})
	})
	expectPanic(t, func() {
		procDocument([]Object{Double{D: 100}})
	})
}

func TestImageMissingPathPanics(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)

	expectPanic(t, func() {
		procImage([]Object{doc, MakeString(filepath.Join(t.TempDir(), "missing.png")), Double{D: 10}, Double{D: 10}})
	})
}

func TestPDFProcsCheckArity(t *testing.T) {
	for name, proc := range map[string]ProcFn{
		"page":       procPage,
		"font":       procFont,
		"font-file":  procFontFile,
		"font-size":  procFontSize,
		"text":       procText,
		"text-wrap":  procTextWrap,
		"line":       procLine,
		"rect":       procRect,
		"oval":       procOval,
		"color":      procColor,
		"stroke":     procStrokeColor,
		"fill":       procFillColor,
		"line-width": procLineWidth,
		"image":      procImage,
		"move-to":    procMoveTo,
		"get-x":      procGetX,
		"get-y":      procGetY,
		"link":       procLink,
		"save":       procSave,
		"to-bytes":   procToBytes,
		"page-count": procPageCount,
		"margins":    procMargins,
	} {
		t.Run(name, func(t *testing.T) {
			expectPanic(t, func() { proc(nil) })
		})
	}
}
