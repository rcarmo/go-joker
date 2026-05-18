package svg

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestCanvasGeneration(t *testing.T) {
	initSVGNamespace()

	// Create a canvas
	canvas := procCanvas([]coretypes.Object{coretypes.MakeInt(200), coretypes.MakeInt(100)})

	// Draw shapes
	procRect([]coretypes.Object{canvas, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(80), coretypes.MakeInt(40)})
	procCircle([]coretypes.Object{canvas, coretypes.MakeInt(150), coretypes.MakeInt(50), coretypes.MakeInt(30)})
	procText([]coretypes.Object{canvas, coretypes.MakeInt(50), coretypes.MakeInt(80), coretypes.MakeString("Hello")})

	// Get SVG string
	result := procToString([]coretypes.Object{canvas})
	svg := result.(coretypes.String).S

	if !strings.Contains(svg, "<svg") {
		t.Fatal("missing <svg tag")
	}
	if !strings.Contains(svg, "<rect") {
		t.Fatal("missing <rect")
	}
	if !strings.Contains(svg, "<circle") {
		t.Fatal("missing <circle")
	}
	if !strings.Contains(svg, "Hello") {
		t.Fatal("missing text")
	}
	if !strings.Contains(svg, "</svg>") {
		t.Fatal("missing closing </svg>")
	}
	t.Logf("SVG length: %d bytes", len(svg))
}

func TestCanvasRejectsInvalidDimensions(t *testing.T) {
	expectSVGPanic(t, func() {
		procCanvas([]coretypes.Object{coretypes.MakeInt(0), coretypes.MakeInt(100)})
	})
	expectSVGPanic(t, func() {
		procCanvasWithViewbox([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(100), coretypes.MakeInt(-1), coretypes.MakeInt(100)})
	})
}

func TestTransformsRejectNonFiniteFloats(t *testing.T) {
	canvas := procCanvas([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})
	expectSVGPanic(t, func() {
		procScale([]coretypes.Object{canvas, coretypes.Double{D: math.Inf(1)}})
	})
	expectSVGPanic(t, func() {
		procScale([]coretypes.Object{canvas, coretypes.Double{D: 1}, coretypes.Double{D: math.NaN()}})
	})
	expectSVGPanic(t, func() {
		procRotate([]coretypes.Object{canvas, coretypes.Double{D: math.Inf(-1)}})
	})
}

func TestShapesRejectInvalidDimensions(t *testing.T) {
	canvas := procCanvas([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})
	expectSVGPanic(t, func() {
		procRect([]coretypes.Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10)})
	})
	expectSVGPanic(t, func() {
		procRoundrect([]coretypes.Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(-1), coretypes.MakeInt(2)})
	})
	expectSVGPanic(t, func() {
		procCircle([]coretypes.Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0)})
	})
	expectSVGPanic(t, func() {
		procEllipse([]coretypes.Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10), coretypes.MakeInt(-1)})
	})
}

func TestCanvasWithStyle(t *testing.T) {
	initSVGNamespace()

	canvas := procCanvas([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})

	style := &ArrayMap{}
	style = style.Assoc(coretypes.MakeKeyword(STRINGS.Intern, "fill"), coretypes.MakeString("red")).(*ArrayMap)
	style = style.Assoc(coretypes.MakeKeyword(STRINGS.Intern, "stroke"), coretypes.MakeString("black")).(*ArrayMap)
	procRect([]coretypes.Object{canvas, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(50), coretypes.MakeInt(50), style})

	result := procToString([]coretypes.Object{canvas})
	svg := result.(coretypes.String).S

	if !strings.Contains(svg, "fill:red") {
		t.Fatalf("missing fill style in: %s", svg)
	}
}

func TestRenderSVG(t *testing.T) {
	initSVGNamespace()

	svgStr := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
		<rect x="0" y="0" width="100" height="100" fill="red"/>
	</svg>`

	img := procRender([]coretypes.Object{coretypes.MakeString(svgStr), coretypes.MakeInt(100), coretypes.MakeInt(100)})
	if img == nil || img == NIL {
		t.Fatal("render returned nil")
	}
	t.Logf("rendered: %s", img.ToString(false))

	expectSVGPanic(t, func() {
		procRender([]coretypes.Object{coretypes.MakeString(svgStr), coretypes.MakeInt(0), coretypes.MakeInt(100)})
	})
}

func expectSVGPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestPolylineRejectsMismatchedCoordinates(t *testing.T) {
	canvas := procCanvas([]coretypes.Object{coretypes.MakeInt(10), coretypes.MakeInt(10)})
	expectSVGPanic(t, func() {
		procPolyline([]coretypes.Object{canvas, NewVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2)), NewVectorFrom(coretypes.MakeInt(1))})
	})
}

func TestPolygonRejectsMissingArgs(t *testing.T) {
	expectSVGPanic(t, func() { procPolygon(nil) })
}

func TestRawChecksArity(t *testing.T) {
	expectSVGPanic(t, func() { procRaw(nil) })
}
