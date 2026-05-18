package pdf

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var pdfNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.pdf"))

func init() {
	pdfNamespace.Lazy = initPDFNamespace
}

func initPDFNamespace() {
	pdfNamespace.ResetMeta(MakeMeta(nil, "PDF document generation. Create PDFs with text, shapes, images, and multi-page support. Backed by signintech/gopdf.", "1.0"))

	procs := []struct {
		name string
		fn   ProcFn
		doc  string
	}{
		// Document
		{"document", procDocument, "Creates a PDF document. Args: :a4 | :a3 | :a5 | :letter | :legal | width height."},
		{"page", procPage, "Adds a new page. Returns document."},
		{"page-count", procPageCount, "Returns number of pages."},
		{"margins", procMargins, "Sets margins: left top right bottom. Returns document."},
		// Fonts
		{"font", procFont, "Sets font by name and size. Args: doc name size. Returns document."},
		{"font-file", procFontFile, "Loads TTF font from file. Args: doc name path [size]. Returns document."},
		{"font-size", procFontSize, "Changes font size. Args: doc size. Returns document."},
		// Text
		{"text", procText, "Draws text at x,y. Args: doc x y string. Returns document."},
		{"text-wrap", procTextWrap, "Draws wrapped text at x,y within width. Args: doc x y width string. Returns document."},
		// Drawing
		{"line", procLine, "Draws a line. Args: doc x1 y1 x2 y2. Returns document."},
		{"rect", procRect, "Draws a rectangle. Args: doc x y w h [:D|:F|:FD]. Returns document."},
		{"oval", procOval, "Draws an oval. Args: doc cx cy rx ry. Returns document."},
		// Color
		{"color", procColor, "Sets text color. Args: doc r g b. Returns document."},
		{"stroke-color", procStrokeColor, "Sets stroke color. Args: doc r g b. Returns document."},
		{"fill-color", procFillColor, "Sets fill color. Args: doc r g b. Returns document."},
		{"line-width", procLineWidth, "Sets line width. Args: doc width. Returns document."},
		// Images
		{"image", procImage, "Places image at x,y. Args: doc path x y [w] [h]. Returns document."},
		// Position
		{"move-to", procMoveTo, "Moves cursor to x,y. Returns document."},
		{"get-x", procGetX, "Returns current x position."},
		{"get-y", procGetY, "Returns current y position."},
		// Links
		{"link", procLink, "Adds a URL link region. Args: doc url x y w h. Returns document."},
		// Output
		{"save", procSave, "Saves PDF to file. Args: doc path."},
		{"to-bytes", procToBytes, "Returns PDF content as byte string."},
	}

	for _, p := range procs {
		pdfNamespace.InternVar(p.name, Proc{Fn: p.fn, Name: "pdf/" + p.name},
			MakeMeta(nil, p.doc, "1.0"))
	}
}
