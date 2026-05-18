package svg

import (
	"bytes"
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"image"
	"image/color"
	"math"
	"os"
	"strings"

	svglib "github.com/ajstarks/svgo"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	. "github.com/rcarmo/go-joker/core"
	imaging "github.com/rcarmo/go-joker/std/imaging"
)

// Canvas wraps an SVG being built.
type Canvas struct {
	coretypes.InfoHolder
	buf *bytes.Buffer
	svg *svglib.SVG
	w   int
	h   int
}

var typeCanvas = &coretypes.Type{}

func (c *Canvas) ToString(escape bool) string {
	return fmt.Sprintf("#<SVG %dx%d>", c.w, c.h)
}
func (c *Canvas) Equals(other interface{}) bool                        { return c == other }
func (c *Canvas) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (c *Canvas) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return c }
func (c *Canvas) GetType() *coretypes.Type                             { return typeCanvas }
func (c *Canvas) Hash() uint32                                         { return 0 }

func extractCanvas(args []coretypes.Object, idx int) *Canvas {
	if idx < 0 || idx >= len(args) {
		panic(RT.NewError("Expected SVG canvas argument"))
	}
	c, ok := args[idx].(*Canvas)
	if !ok {
		panic(RT.NewError("Expected SVG canvas argument"))
	}
	return c
}

// parseStyle converts a Joker map to SVG style string
func parseStyle(args []coretypes.Object, idx int) string {
	if idx >= len(args) {
		return ""
	}
	m, ok := args[idx].(coretypes.Map)
	if !ok {
		// Try as a string
		if s, ok := args[idx].(coretypes.String); ok {
			return s.S
		}
		return ""
	}
	var parts []string
	for iter := m.Iter(); iter.HasNext(); {
		pair := iter.Next()
		k := pair.Key.ToString(false)
		// Strip leading : from keywords
		if len(k) > 0 && k[0] == ':' {
			k = k[1:]
		}
		v := pair.Value.ToString(false)
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ";")
}

// --- Creation ---

func positiveDimension(obj coretypes.Object, context, name string) int {
	v := coretypes.EnsureObjectIsInt(obj, context+" "+name+": %s").I
	if v <= 0 {
		panic(RT.NewError(context + ": " + name + " must be positive"))
	}
	return v
}

func nonNegativeDimension(obj coretypes.Object, context, name string) int {
	v := coretypes.EnsureObjectIsInt(obj, context+" "+name+": %s").I
	if v < 0 {
		panic(RT.NewError(context + ": " + name + " must be non-negative"))
	}
	return v
}

func finiteSVGFloat(obj coretypes.Object, context, name string) float64 {
	v := ExtractDouble([]coretypes.Object{obj}, 0)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		panic(RT.NewError(context + ": " + name + " must be finite"))
	}
	return v
}

var procCanvas ProcFn = func(args []coretypes.Object) coretypes.Object {
	w := positiveDimension(args[0], "svg/canvas", "width")
	h := positiveDimension(args[1], "svg/canvas", "height")
	buf := &bytes.Buffer{}
	s := svglib.New(buf)
	s.Start(w, h)
	return &Canvas{buf: buf, svg: s, w: w, h: h}
}

var procCanvasWithViewbox ProcFn = func(args []coretypes.Object) coretypes.Object {
	w := positiveDimension(args[0], "svg/canvas", "width")
	h := positiveDimension(args[1], "svg/canvas", "height")
	vw := positiveDimension(args[2], "svg/canvas", "viewbox width")
	vh := positiveDimension(args[3], "svg/canvas", "viewbox height")
	buf := &bytes.Buffer{}
	s := svglib.New(buf)
	s.Startview(w, h, 0, 0, vw, vh)
	return &Canvas{buf: buf, svg: s, w: w, h: h}
}

// --- Shapes ---

var procRect ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	x := ExtractInt(args, 1)
	y := ExtractInt(args, 2)
	w := positiveDimension(args[3], "svg/rect", "width")
	h := positiveDimension(args[4], "svg/rect", "height")
	style := parseStyle(args, 5)
	if style != "" {
		c.svg.Rect(x, y, w, h, "style=\""+style+"\"")
	} else {
		c.svg.Rect(x, y, w, h)
	}
	return args[0]
}

var procRoundrect ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	x := ExtractInt(args, 1)
	y := ExtractInt(args, 2)
	w := positiveDimension(args[3], "svg/roundrect", "width")
	h := positiveDimension(args[4], "svg/roundrect", "height")
	rx := nonNegativeDimension(args[5], "svg/roundrect", "rx")
	ry := nonNegativeDimension(args[6], "svg/roundrect", "ry")
	style := parseStyle(args, 7)
	if style != "" {
		c.svg.Roundrect(x, y, w, h, rx, ry, "style=\""+style+"\"")
	} else {
		c.svg.Roundrect(x, y, w, h, rx, ry)
	}
	return args[0]
}

var procCircle ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	cx := ExtractInt(args, 1)
	cy := ExtractInt(args, 2)
	r := positiveDimension(args[3], "svg/circle", "radius")
	style := parseStyle(args, 4)
	if style != "" {
		c.svg.Circle(cx, cy, r, "style=\""+style+"\"")
	} else {
		c.svg.Circle(cx, cy, r)
	}
	return args[0]
}

var procEllipse ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	cx := ExtractInt(args, 1)
	cy := ExtractInt(args, 2)
	rx := positiveDimension(args[3], "svg/ellipse", "rx")
	ry := positiveDimension(args[4], "svg/ellipse", "ry")
	style := parseStyle(args, 5)
	if style != "" {
		c.svg.Ellipse(cx, cy, rx, ry, "style=\""+style+"\"")
	} else {
		c.svg.Ellipse(cx, cy, rx, ry)
	}
	return args[0]
}

var procLine ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	x1 := ExtractInt(args, 1)
	y1 := ExtractInt(args, 2)
	x2 := ExtractInt(args, 3)
	y2 := ExtractInt(args, 4)
	style := parseStyle(args, 5)
	if style != "" {
		c.svg.Line(x1, y1, x2, y2, "style=\""+style+"\"")
	} else {
		c.svg.Line(x1, y1, x2, y2)
	}
	return args[0]
}

var procPath ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	d := ExtractString(args, 1)
	style := parseStyle(args, 2)
	if style != "" {
		c.svg.Path(d, "style=\""+style+"\"")
	} else {
		c.svg.Path(d)
	}
	return args[0]
}

var procPolygon ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 4)
	c := extractCanvas(args, 0)
	// args[1] = vector of x coords, args[2] = vector of y coords
	xv, ok := args[1].(coretypes.Indexed)
	if !ok {
		panic(RT.NewError("svg/polygon: x coordinates must be indexed"))
	}
	yv, ok := args[2].(coretypes.Indexed)
	if !ok {
		panic(RT.NewError("svg/polygon: y coordinates must be indexed"))
	}
	xc, ok := args[1].(coretypes.Counted)
	if !ok {
		panic(RT.NewError("svg/polygon: x coordinates must be counted"))
	}
	yc, ok := args[2].(coretypes.Counted)
	if !ok {
		panic(RT.NewError("svg/polygon: y coordinates must be counted"))
	}
	n := xc.Count()
	if yc.Count() != n {
		panic(RT.NewError("svg/polygon: coordinate vectors must have equal length"))
	}
	xs := make([]int, n)
	ys := make([]int, n)
	for i := 0; i < n; i++ {
		xs[i] = coretypes.EnsureObjectIsInt(xv.Nth(i), "").I
		ys[i] = coretypes.EnsureObjectIsInt(yv.Nth(i), "").I
	}
	style := parseStyle(args, 3)
	if style != "" {
		c.svg.Polygon(xs, ys, "style=\""+style+"\"")
	} else {
		c.svg.Polygon(xs, ys)
	}
	return args[0]
}

var procPolyline ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 4)
	c := extractCanvas(args, 0)
	xv, ok := args[1].(coretypes.Indexed)
	if !ok {
		panic(RT.NewError("svg/polyline: x coordinates must be indexed"))
	}
	yv, ok := args[2].(coretypes.Indexed)
	if !ok {
		panic(RT.NewError("svg/polyline: y coordinates must be indexed"))
	}
	xc, ok := args[1].(coretypes.Counted)
	if !ok {
		panic(RT.NewError("svg/polyline: x coordinates must be counted"))
	}
	yc, ok := args[2].(coretypes.Counted)
	if !ok {
		panic(RT.NewError("svg/polyline: y coordinates must be counted"))
	}
	n := xc.Count()
	if yc.Count() != n {
		panic(RT.NewError("svg/polyline: coordinate vectors must have equal length"))
	}
	xs := make([]int, n)
	ys := make([]int, n)
	for i := 0; i < n; i++ {
		xs[i] = coretypes.EnsureObjectIsInt(xv.Nth(i), "").I
		ys[i] = coretypes.EnsureObjectIsInt(yv.Nth(i), "").I
	}
	style := parseStyle(args, 3)
	if style != "" {
		c.svg.Polyline(xs, ys, "style=\""+style+"\"")
	} else {
		c.svg.Polyline(xs, ys)
	}
	return args[0]
}

// --- Text ---

var procText ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	x := ExtractInt(args, 1)
	y := ExtractInt(args, 2)
	text := ExtractString(args, 3)
	style := parseStyle(args, 4)
	if style != "" {
		c.svg.Text(x, y, text, "style=\""+style+"\"")
	} else {
		c.svg.Text(x, y, text)
	}
	return args[0]
}

// --- Grouping & Transform ---

var procGroup ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	style := parseStyle(args, 1)
	if style != "" {
		c.svg.Group("style=\"" + style + "\"")
	} else {
		c.svg.Group()
	}
	return args[0]
}

var procGroupEnd ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	c.svg.Gend()
	return args[0]
}

var procTranslate ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	x := ExtractInt(args, 1)
	y := ExtractInt(args, 2)
	c.svg.Gtransform(fmt.Sprintf("translate(%d,%d)", x, y))
	return args[0]
}

var procScale ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	sx := finiteSVGFloat(args[1], "svg/scale", "sx")
	sy := sx
	if len(args) > 2 {
		sy = finiteSVGFloat(args[2], "svg/scale", "sy")
	}
	c.svg.Gtransform(fmt.Sprintf("scale(%g,%g)", sx, sy))
	return args[0]
}

var procRotate ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	angle := finiteSVGFloat(args[1], "svg/rotate", "angle")
	c.svg.Gtransform(fmt.Sprintf("rotate(%g)", angle))
	return args[0]
}

var procTransformEnd ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	c.svg.Gend()
	return args[0]
}

// --- Definitions & Use ---

var procDef ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	c.svg.Def()
	return args[0]
}

var procDefEnd ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	c.svg.DefEnd()
	return args[0]
}

// --- Output ---

var procToString ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	c.svg.End()
	return coretypes.MakeString(c.buf.String())
}

var procSave ProcFn = func(args []coretypes.Object) coretypes.Object {
	c := extractCanvas(args, 0)
	path := ExtractString(args, 1)
	c.svg.End()
	err := writeFile(path, c.buf.Bytes())
	if err != nil {
		panic(RT.NewError("svg/save: " + err.Error()))
	}
	return NIL
}

// --- Raw SVG injection ---

var procRaw ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	c := extractCanvas(args, 0)
	s := ExtractString(args, 1)
	if _, err := fmt.Fprint(c.buf, s); err != nil {
		panic(RT.NewError("svg/raw: " + err.Error()))
	}
	return args[0]
}

// --- Render SVG to raster Image ---

func renderDimension(obj coretypes.Object, name string) int {
	return positiveDimension(obj, "svg/render", name)
}

func rgbaToNRGBA(img *image.RGBA, w, h int) *image.NRGBA {
	nrgba := image.NewNRGBA(img.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 {
				nrgba.SetNRGBA(x, y, color.NRGBA{
					R: uint8(r * 255 / a),
					G: uint8(g * 255 / a),
					B: uint8(b * 255 / a),
					A: uint8(a >> 8),
				})
			}
		}
	}
	return nrgba
}

var procRender ProcFn = func(args []coretypes.Object) coretypes.Object {
	svgData := ExtractString(args, 0)
	w := renderDimension(args[1], "width")
	h := renderDimension(args[2], "height")

	icon, err := oksvg.ReadIconStream(strings.NewReader(svgData))
	if err != nil {
		panic(RT.NewError("svg/render: " + err.Error()))
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	dasher := rasterx.NewDasher(w, h, scanner)
	icon.Draw(dasher, 1.0)

	// Convert RGBA to NRGBA for imaging compatibility
	return imaging.WrapImage(rgbaToNRGBA(img, w, h))
}

// --- Render SVG file to raster ---

var procRenderFile ProcFn = func(args []coretypes.Object) coretypes.Object {
	path := ExtractString(args, 0)
	w := renderDimension(args[1], "width")
	h := renderDimension(args[2], "height")

	icon, err := oksvg.ReadIcon(path, oksvg.StrictErrorMode)
	if err != nil {
		panic(RT.NewError("svg/render-file: " + err.Error()))
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	dasher := rasterx.NewDasher(w, h, scanner)
	icon.Draw(dasher, 1.0)

	return imaging.WrapImage(rgbaToNRGBA(img, w, h))
}

// --- Helpers ---

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
