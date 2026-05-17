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
	canvas := procCanvas([]Object{coretypes.MakeInt(200), coretypes.MakeInt(100)})

	// Draw shapes
	procRect([]Object{canvas, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(80), coretypes.MakeInt(40)})
	procCircle([]Object{canvas, coretypes.MakeInt(150), coretypes.MakeInt(50), coretypes.MakeInt(30)})
	procText([]Object{canvas, coretypes.MakeInt(50), coretypes.MakeInt(80), MakeString("Hello")})

	// Get SVG string
	result := procToString([]Object{canvas})
	svg := result.(String).S

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
		procCanvas([]Object{coretypes.MakeInt(0), coretypes.MakeInt(100)})
	})
	expectSVGPanic(t, func() {
		procCanvasWithViewbox([]Object{coretypes.MakeInt(100), coretypes.MakeInt(100), coretypes.MakeInt(-1), coretypes.MakeInt(100)})
	})
}

func TestTransformsRejectNonFiniteFloats(t *testing.T) {
	canvas := procCanvas([]Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})
	expectSVGPanic(t, func() {
		procScale([]Object{canvas, Double{D: math.Inf(1)}})
	})
	expectSVGPanic(t, func() {
		procScale([]Object{canvas, Double{D: 1}, Double{D: math.NaN()}})
	})
	expectSVGPanic(t, func() {
		procRotate([]Object{canvas, Double{D: math.Inf(-1)}})
	})
}

func TestShapesRejectInvalidDimensions(t *testing.T) {
	canvas := procCanvas([]Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})
	expectSVGPanic(t, func() {
		procRect([]Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10)})
	})
	expectSVGPanic(t, func() {
		procRoundrect([]Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(-1), coretypes.MakeInt(2)})
	})
	expectSVGPanic(t, func() {
		procCircle([]Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0)})
	})
	expectSVGPanic(t, func() {
		procEllipse([]Object{canvas, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(10), coretypes.MakeInt(-1)})
	})
}

func TestCanvasWithStyle(t *testing.T) {
	initSVGNamespace()

	canvas := procCanvas([]Object{coretypes.MakeInt(100), coretypes.MakeInt(100)})

	style := &ArrayMap{}
	style = style.Assoc(MakeKeyword("fill"), MakeString("red")).(*ArrayMap)
	style = style.Assoc(MakeKeyword("stroke"), MakeString("black")).(*ArrayMap)
	procRect([]Object{canvas, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(50), coretypes.MakeInt(50), style})

	result := procToString([]Object{canvas})
	svg := result.(String).S

	if !strings.Contains(svg, "fill:red") {
		t.Fatalf("missing fill style in: %s", svg)
	}
}

func TestRenderSVG(t *testing.T) {
	initSVGNamespace()

	svgStr := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
		<rect x="0" y="0" width="100" height="100" fill="red"/>
	</svg>`

	img := procRender([]Object{MakeString(svgStr), coretypes.MakeInt(100), coretypes.MakeInt(100)})
	if img == nil || img == NIL {
		t.Fatal("render returned nil")
	}
	t.Logf("rendered: %s", img.ToString(false))

	expectSVGPanic(t, func() {
		procRender([]Object{MakeString(svgStr), coretypes.MakeInt(0), coretypes.MakeInt(100)})
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
	canvas := procCanvas([]Object{coretypes.MakeInt(10), coretypes.MakeInt(10)})
	expectSVGPanic(t, func() {
		procPolyline([]Object{canvas, NewVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2)), NewVectorFrom(coretypes.MakeInt(1))})
	})
}

func TestPolygonRejectsMissingArgs(t *testing.T) {
	expectSVGPanic(t, func() { procPolygon(nil) })
}

func TestRawChecksArity(t *testing.T) {
	expectSVGPanic(t, func() { procRaw(nil) })
}
