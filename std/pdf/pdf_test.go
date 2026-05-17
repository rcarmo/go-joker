package pdf

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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

	doc := procDocument([]coretypes.Object{MakeKeyword("a4")})
	if doc == nil {
		t.Fatal("document creation failed")
	}
	t.Logf("doc: %s", doc.ToString(false))

	// Add text (need a font first)
	// gopdf requires an explicit TTF font - skip text test without font file

	// Draw shapes (these work without fonts)
	procLine([]coretypes.Object{doc, coretypes.Double{D: 50}, coretypes.Double{D: 50}, coretypes.Double{D: 500}, coretypes.Double{D: 50}})
	procRect([]coretypes.Object{doc, coretypes.Double{D: 50}, coretypes.Double{D: 100}, coretypes.Double{D: 200}, coretypes.Double{D: 150}})
	procOval([]coretypes.Object{doc, coretypes.Double{D: 300}, coretypes.Double{D: 200}, coretypes.Double{D: 50}, coretypes.Double{D: 30}})

	// Color
	procStrokeColor([]coretypes.Object{doc, coretypes.MakeInt(255), coretypes.MakeInt(0), coretypes.MakeInt(0)})
	procLine([]coretypes.Object{doc, coretypes.Double{D: 50}, coretypes.Double{D: 300}, coretypes.Double{D: 500}, coretypes.Double{D: 300}})

	// New page
	procPage([]coretypes.Object{doc})
	count := procPageCount([]coretypes.Object{doc})
	if count.(coretypes.Int).I != 2 {
		t.Fatalf("expected 2 pages, got %v", count)
	}

	// Save
	path := filepath.Join(t.TempDir(), "test-output.pdf")
	procSave([]coretypes.Object{doc, coretypes.MakeString(path)})

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
		procColor([]coretypes.Object{doc, coretypes.MakeInt(256), coretypes.MakeInt(0), coretypes.MakeInt(0)})
	})
	expectPanic(t, func() {
		procStrokeColor([]coretypes.Object{doc, coretypes.MakeInt(0), coretypes.MakeInt(-1), coretypes.MakeInt(0)})
	})
	expectPanic(t, func() {
		procFillColor([]coretypes.Object{doc, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(999)})
	})
}

func TestPDFRejectsInvalidFiniteNumbers(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procFontSize([]coretypes.Object{doc, coretypes.Double{D: math.Inf(1)}})
	})
	expectPanic(t, func() {
		procLine([]coretypes.Object{doc, coretypes.Double{D: math.NaN()}, coretypes.Double{D: 0}, coretypes.Double{D: 10}, coretypes.Double{D: 10}})
	})
	expectPanic(t, func() {
		procMoveTo([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: math.Inf(-1)}})
	})
}

func TestPDFGeometryRejectsNonFiniteDimensions(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procRect([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: math.Inf(1)}, coretypes.Double{D: 10}})
	})
	expectPanic(t, func() {
		procMargins([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: math.NaN()}, coretypes.Double{D: 10}, coretypes.Double{D: 10}})
	})
}

func TestPDFGeometryRejectsInvalidDimensions(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procTextWrap([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: 0}, coretypes.MakeString("x")})
	})
	expectPanic(t, func() {
		procRect([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: -1}, coretypes.Double{D: 10}})
	})
	expectPanic(t, func() {
		procOval([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: 1}, coretypes.Double{D: 0}})
	})
	expectPanic(t, func() {
		procImage([]coretypes.Object{doc, coretypes.MakeString(filepath.Join(t.TempDir(), "missing.png")), coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: -1}})
	})
	expectPanic(t, func() {
		procLink([]coretypes.Object{doc, coretypes.MakeString("https://example.com"), coretypes.Double{D: 10}, coretypes.Double{D: 10}, coretypes.Double{D: 0}, coretypes.Double{D: 10}})
	})
	expectPanic(t, func() {
		procMargins([]coretypes.Object{doc, coretypes.Double{D: 10}, coretypes.Double{D: -1}, coretypes.Double{D: 10}, coretypes.Double{D: 10}})
	})
}

func TestPDFLineWidthRejectsInvalidValue(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)
	expectPanic(t, func() {
		procLineWidth([]coretypes.Object{doc, coretypes.Double{D: 0}})
	})
}

func TestPDFDocumentRejectsInvalidDimensions(t *testing.T) {
	initPDFNamespace()
	expectPanic(t, func() {
		procDocument([]coretypes.Object{coretypes.Double{D: 0}, coretypes.Double{D: 100}})
	})
	expectPanic(t, func() {
		procDocument([]coretypes.Object{coretypes.Double{D: 100}, coretypes.Double{D: -1}})
	})
	expectPanic(t, func() {
		procDocument([]coretypes.Object{MakeKeyword("bogus")})
	})
	expectPanic(t, func() {
		procDocument([]coretypes.Object{coretypes.Double{D: 100}})
	})
}

func TestImageMissingPathPanics(t *testing.T) {
	initPDFNamespace()
	doc := procDocument(nil)

	expectPanic(t, func() {
		procImage([]coretypes.Object{doc, coretypes.MakeString(filepath.Join(t.TempDir(), "missing.png")), coretypes.Double{D: 10}, coretypes.Double{D: 10}})
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
