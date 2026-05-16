package pdf

import (
	"bytes"
	"fmt"

	"github.com/signintech/gopdf"

	. "github.com/rcarmo/go-joker/core"
)

// Document wraps a gopdf instance.
type Document struct {
	InfoHolder
	pdf    *gopdf.GoPdf
	w, h   float64
	closed bool
}

var typeDocument = &Type{}

func (d *Document) ToString(escape bool) string {
	return fmt.Sprintf("#<PDF %.0fx%.0f>", d.w, d.h)
}
func (d *Document) Equals(other interface{}) bool    { return d == other }
func (d *Document) GetInfo() *ObjectInfo             { return nil }
func (d *Document) WithInfo(info *ObjectInfo) Object { return d }
func (d *Document) GetType() *Type                   { return typeDocument }
func (d *Document) Hash() uint32                     { return 0 }

func extractDoc(args []Object, idx int) *Document {
	if idx < 0 || idx >= len(args) {
		panic(RT.NewError("Expected PDF document argument"))
	}
	d, ok := args[idx].(*Document)
	if !ok {
		panic(RT.NewError("Expected PDF document argument"))
	}
	return d
}

// --- Page sizes ---

var pageSizes = map[string]*gopdf.Rect{
	"a4":        gopdf.PageSizeA4,
	"a3":        gopdf.PageSizeA3,
	"a5":        gopdf.PageSizeA5,
	"letter":    gopdf.PageSizeLetter,
	"legal":     gopdf.PageSizeLegal,
	"landscape": {W: 842, H: 595}, // A4 landscape
}

// --- Creation ---

func positivePDFDimension(v float64, name string) float64 {
	if v <= 0 {
		panic(RT.NewError("pdf: " + name + " must be positive"))
	}
	return v
}

func nonNegativePDFDimension(v float64, name string) float64 {
	if v < 0 {
		panic(RT.NewError("pdf: " + name + " must be non-negative"))
	}
	return v
}

var procDocument ProcFn = func(args []Object) Object {
	CheckArity(args, 0, 2)
	w := 595.0 // A4 default
	h := 842.0
	if len(args) > 0 {
		if kw, ok := args[0].(Keyword); ok {
			size := pageSizes[kw.Name()]
			if size != nil {
				w, h = size.W, size.H
			}
		} else if len(args) >= 2 {
			w = positivePDFDimension(ExtractDouble(args, 0), "width")
			h = positivePDFDimension(ExtractDouble(args, 1), "height")
		}
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: w, H: h}})
	pdf.AddPage()
	return &Document{pdf: pdf, w: w, h: h}
}

var procPage ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	d := extractDoc(args, 0)
	d.pdf.AddPage()
	return args[0]
}

// --- Fonts ---

var procFont ProcFn = func(args []Object) Object {
	CheckArity(args, 3, 3)
	d := extractDoc(args, 0)
	name := ExtractString(args, 1)
	size := ExtractDouble(args, 2)

	// Check if font already added, if not try to add it
	err := d.pdf.SetFont(name, "", int(size))
	if err != nil {
		// Try to add as TTF file path
		err2 := d.pdf.AddTTFFont(name, name)
		if err2 != nil {
			panic(RT.NewError("pdf/font: " + err.Error() + " (also tried as file: " + err2.Error() + ")"))
		}
		err = d.pdf.SetFont(name, "", int(size))
		if err != nil {
			panic(RT.NewError("pdf/font: " + err.Error()))
		}
	}
	return args[0]
}

var procFontFile ProcFn = func(args []Object) Object {
	CheckArity(args, 3, 4)
	d := extractDoc(args, 0)
	name := ExtractString(args, 1)
	path := ExtractString(args, 2)
	size := 12.0
	if len(args) > 3 {
		size = ExtractDouble(args, 3)
	}
	err := d.pdf.AddTTFFont(name, path)
	if err != nil {
		panic(RT.NewError("pdf/font-file: " + err.Error()))
	}
	err = d.pdf.SetFont(name, "", int(size))
	if err != nil {
		panic(RT.NewError("pdf/font-file set: " + err.Error()))
	}
	return args[0]
}

var procFontSize ProcFn = func(args []Object) Object {
	CheckArity(args, 2, 2)
	d := extractDoc(args, 0)
	size := ExtractDouble(args, 1)
	if err := d.pdf.SetFontSize(size); err != nil {
		panic(RT.NewError("pdf/font-size: " + err.Error()))
	}
	return args[0]
}

// --- Text ---

var procText ProcFn = func(args []Object) Object {
	CheckArity(args, 4, 4)
	d := extractDoc(args, 0)
	x := ExtractDouble(args, 1)
	y := ExtractDouble(args, 2)
	text := ExtractString(args, 3)
	d.pdf.SetXY(x, y)
	if err := d.pdf.Cell(nil, text); err != nil {
		panic(RT.NewError("pdf/text: " + err.Error()))
	}
	return args[0]
}

var procTextWrap ProcFn = func(args []Object) Object {
	CheckArity(args, 5, 5)
	d := extractDoc(args, 0)
	x := ExtractDouble(args, 1)
	y := ExtractDouble(args, 2)
	w := positivePDFDimension(ExtractDouble(args, 3), "text width")
	text := ExtractString(args, 4)
	d.pdf.SetXY(x, y)
	rect := &gopdf.Rect{W: w, H: 0}
	if err := d.pdf.MultiCell(rect, text); err != nil {
		panic(RT.NewError("pdf/text-wrap: " + err.Error()))
	}
	return args[0]
}

// --- Drawing ---

var procLine ProcFn = func(args []Object) Object {
	CheckArity(args, 5, 5)
	d := extractDoc(args, 0)
	x1 := ExtractDouble(args, 1)
	y1 := ExtractDouble(args, 2)
	x2 := ExtractDouble(args, 3)
	y2 := ExtractDouble(args, 4)
	d.pdf.Line(x1, y1, x2, y2)
	return args[0]
}

var procRect ProcFn = func(args []Object) Object {
	CheckArity(args, 5, 6)
	d := extractDoc(args, 0)
	x := ExtractDouble(args, 1)
	y := ExtractDouble(args, 2)
	w := positivePDFDimension(ExtractDouble(args, 3), "rect width")
	h := positivePDFDimension(ExtractDouble(args, 4), "rect height")
	style := "D" // draw border
	if len(args) > 5 {
		style = ExtractKeyword(args, 5)
	}
	d.pdf.RectFromUpperLeftWithStyle(x, y, w, h, style)
	return args[0]
}

var procOval ProcFn = func(args []Object) Object {
	CheckArity(args, 5, 5)
	d := extractDoc(args, 0)
	x := ExtractDouble(args, 1)
	y := ExtractDouble(args, 2)
	rx := positivePDFDimension(ExtractDouble(args, 3), "oval rx")
	ry := positivePDFDimension(ExtractDouble(args, 4), "oval ry")
	d.pdf.Oval(x, y, rx, ry)
	return args[0]
}

// --- Color ---

func pdfColorChannel(obj Object, name string) uint8 {
	v := EnsureObjectIsInt(obj, "pdf color "+name+": %s").I
	if v < 0 || v > 255 {
		panic(RT.NewError("pdf color " + name + " must be in [0,255]"))
	}
	return uint8(v)
}

var procColor ProcFn = func(args []Object) Object {
	CheckArity(args, 4, 4)
	d := extractDoc(args, 0)
	r := pdfColorChannel(args[1], "r")
	g := pdfColorChannel(args[2], "g")
	b := pdfColorChannel(args[3], "b")
	d.pdf.SetTextColor(r, g, b)
	return args[0]
}

var procStrokeColor ProcFn = func(args []Object) Object {
	CheckArity(args, 4, 4)
	d := extractDoc(args, 0)
	r := pdfColorChannel(args[1], "r")
	g := pdfColorChannel(args[2], "g")
	b := pdfColorChannel(args[3], "b")
	d.pdf.SetStrokeColor(r, g, b)
	return args[0]
}

var procFillColor ProcFn = func(args []Object) Object {
	CheckArity(args, 4, 4)
	d := extractDoc(args, 0)
	r := pdfColorChannel(args[1], "r")
	g := pdfColorChannel(args[2], "g")
	b := pdfColorChannel(args[3], "b")
	d.pdf.SetFillColor(r, g, b)
	return args[0]
}

var procLineWidth ProcFn = func(args []Object) Object {
	CheckArity(args, 2, 2)
	d := extractDoc(args, 0)
	w := positivePDFDimension(ExtractDouble(args, 1), "line width")
	d.pdf.SetLineWidth(w)
	return args[0]
}

// --- Images ---

var procImage ProcFn = func(args []Object) Object {
	CheckArity(args, 4, 6)
	d := extractDoc(args, 0)
	path := ExtractString(args, 1)
	x := ExtractDouble(args, 2)
	y := ExtractDouble(args, 3)

	opts := &gopdf.Rect{}
	if len(args) > 4 {
		opts.W = positivePDFDimension(ExtractDouble(args, 4), "image width")
	}
	if len(args) > 5 {
		opts.H = positivePDFDimension(ExtractDouble(args, 5), "image height")
	}

	if err := d.pdf.Image(path, x, y, opts); err != nil {
		panic(RT.NewError("pdf/image: " + err.Error()))
	}
	return args[0]
}

// --- Position ---

var procMoveTo ProcFn = func(args []Object) Object {
	CheckArity(args, 3, 3)
	d := extractDoc(args, 0)
	x := ExtractDouble(args, 1)
	y := ExtractDouble(args, 2)
	d.pdf.SetXY(x, y)
	return args[0]
}

var procGetX ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	d := extractDoc(args, 0)
	return Double{D: d.pdf.GetX()}
}

var procGetY ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	d := extractDoc(args, 0)
	return Double{D: d.pdf.GetY()}
}

// --- Link ---

var procLink ProcFn = func(args []Object) Object {
	CheckArity(args, 6, 6)
	d := extractDoc(args, 0)
	url := ExtractString(args, 1)
	x := ExtractDouble(args, 2)
	y := ExtractDouble(args, 3)
	w := positivePDFDimension(ExtractDouble(args, 4), "link width")
	h := positivePDFDimension(ExtractDouble(args, 5), "link height")
	d.pdf.AddExternalLink(url, x, y, w, h)
	return args[0]
}

// --- Output ---

var procSave ProcFn = func(args []Object) Object {
	CheckArity(args, 2, 2)
	d := extractDoc(args, 0)
	path := ExtractString(args, 1)
	err := d.pdf.WritePdf(path)
	if err != nil {
		panic(RT.NewError("pdf/save: " + err.Error()))
	}
	return NIL
}

var procToBytes ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	d := extractDoc(args, 0)
	var buf bytes.Buffer
	_, err := d.pdf.WriteTo(&buf)
	if err != nil {
		panic(RT.NewError("pdf/to-bytes: " + err.Error()))
	}
	return MakeString(buf.String())
}

// --- Page info ---

var procPageCount ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	d := extractDoc(args, 0)
	return MakeInt(d.pdf.GetNumberOfPages())
}

// --- Margins ---

var procMargins ProcFn = func(args []Object) Object {
	CheckArity(args, 5, 5)
	d := extractDoc(args, 0)
	left := nonNegativePDFDimension(ExtractDouble(args, 1), "left margin")
	top := nonNegativePDFDimension(ExtractDouble(args, 2), "top margin")
	right := nonNegativePDFDimension(ExtractDouble(args, 3), "right margin")
	bottom := nonNegativePDFDimension(ExtractDouble(args, 4), "bottom margin")
	d.pdf.SetMargins(left, top, right, bottom)
	return args[0]
}
