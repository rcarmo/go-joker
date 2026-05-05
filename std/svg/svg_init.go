package svg

import (
	. "github.com/candid82/joker/core"
)

var svgNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("joker.svg"))

func init() {
	svgNamespace.Lazy = initSVGNamespace
}

func initSVGNamespace() {
	svgNamespace.ResetMeta(MakeMeta(nil, "SVG generation and rendering. Create SVGs programmatically and render them to raster images.", "1.0"))

	procs := []struct {
		name string
		fn   ProcFn
		doc  string
	}{
		// Creation
		{"canvas", procCanvas, "Creates an SVG canvas with width and height."},
		{"canvas-viewbox", procCanvasWithViewbox, "Creates an SVG canvas with width, height, viewbox-width, viewbox-height."},
		// Shapes
		{"rect", procRect, "Draws a rectangle. Args: canvas x y w h [style-map]. Returns canvas."},
		{"roundrect", procRoundrect, "Draws a rounded rectangle. Args: canvas x y w h rx ry [style-map]. Returns canvas."},
		{"circle", procCircle, "Draws a circle. Args: canvas cx cy r [style-map]. Returns canvas."},
		{"ellipse", procEllipse, "Draws an ellipse. Args: canvas cx cy rx ry [style-map]. Returns canvas."},
		{"line", procLine, "Draws a line. Args: canvas x1 y1 x2 y2 [style-map]. Returns canvas."},
		{"path", procPath, "Draws a path. Args: canvas d-string [style-map]. Returns canvas."},
		{"polygon", procPolygon, "Draws a polygon. Args: canvas xs-vec ys-vec [style-map]. Returns canvas."},
		{"polyline", procPolyline, "Draws a polyline. Args: canvas xs-vec ys-vec [style-map]. Returns canvas."},
		// Text
		{"text", procText, "Draws text. Args: canvas x y string [style-map]. Returns canvas."},
		// Grouping
		{"group", procGroup, "Starts a group. Args: canvas [style-map]. Returns canvas."},
		{"group-end", procGroupEnd, "Ends a group. Returns canvas."},
		{"translate", procTranslate, "Starts a translate transform group. Args: canvas x y. Returns canvas."},
		{"scale", procScale, "Starts a scale transform group. Args: canvas sx [sy]. Returns canvas."},
		{"rotate", procRotate, "Starts a rotate transform group. Args: canvas degrees. Returns canvas."},
		{"transform-end", procTransformEnd, "Ends a transform group. Returns canvas."},
		// Defs
		{"def", procDef, "Starts a defs section. Returns canvas."},
		{"def-end", procDefEnd, "Ends a defs section. Returns canvas."},
		// Output
		{"to-string", procToString, "Finalizes SVG and returns as string."},
		{"save", procSave, "Finalizes SVG and saves to file. Args: canvas path."},
		{"raw", procRaw, "Injects raw SVG markup. Args: canvas string. Returns canvas."},
		// Rendering
		{"render", procRender, "Renders SVG string to a raster Image at width×height."},
		{"render-file", procRenderFile, "Renders SVG file to a raster Image at width×height."},
	}

	for _, p := range procs {
		svgNamespace.InternVar(p.name, Proc{Fn: p.fn, Name: "svg/" + p.name},
			MakeMeta(nil, p.doc, "1.0"))
	}
}
