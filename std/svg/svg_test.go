package svg

import (
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestCanvasGeneration(t *testing.T) {
	initSVGNamespace()

	// Create a canvas
	canvas := procCanvas([]Object{MakeInt(200), MakeInt(100)})

	// Draw shapes
	procRect([]Object{canvas, MakeInt(10), MakeInt(10), MakeInt(80), MakeInt(40)})
	procCircle([]Object{canvas, MakeInt(150), MakeInt(50), MakeInt(30)})
	procText([]Object{canvas, MakeInt(50), MakeInt(80), MakeString("Hello")})

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
		procCanvas([]Object{MakeInt(0), MakeInt(100)})
	})
	expectSVGPanic(t, func() {
		procCanvasWithViewbox([]Object{MakeInt(100), MakeInt(100), MakeInt(-1), MakeInt(100)})
	})
}

func TestShapesRejectInvalidDimensions(t *testing.T) {
	canvas := procCanvas([]Object{MakeInt(100), MakeInt(100)})
	expectSVGPanic(t, func() {
		procRect([]Object{canvas, MakeInt(0), MakeInt(0), MakeInt(0), MakeInt(10)})
	})
	expectSVGPanic(t, func() {
		procRoundrect([]Object{canvas, MakeInt(0), MakeInt(0), MakeInt(10), MakeInt(10), MakeInt(-1), MakeInt(2)})
	})
	expectSVGPanic(t, func() {
		procCircle([]Object{canvas, MakeInt(0), MakeInt(0), MakeInt(0)})
	})
	expectSVGPanic(t, func() {
		procEllipse([]Object{canvas, MakeInt(0), MakeInt(0), MakeInt(10), MakeInt(-1)})
	})
}

func TestCanvasWithStyle(t *testing.T) {
	initSVGNamespace()

	canvas := procCanvas([]Object{MakeInt(100), MakeInt(100)})

	style := &ArrayMap{}
	style = style.Assoc(MakeKeyword("fill"), MakeString("red")).(*ArrayMap)
	style = style.Assoc(MakeKeyword("stroke"), MakeString("black")).(*ArrayMap)
	procRect([]Object{canvas, MakeInt(10), MakeInt(10), MakeInt(50), MakeInt(50), style})

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

	img := procRender([]Object{MakeString(svgStr), MakeInt(100), MakeInt(100)})
	if img == nil || img == NIL {
		t.Fatal("render returned nil")
	}
	t.Logf("rendered: %s", img.ToString(false))

	expectSVGPanic(t, func() {
		procRender([]Object{MakeString(svgStr), MakeInt(0), MakeInt(100)})
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
	canvas := procCanvas([]Object{MakeInt(10), MakeInt(10)})
	expectSVGPanic(t, func() {
		procPolyline([]Object{canvas, NewVectorFrom(MakeInt(1), MakeInt(2)), NewVectorFrom(MakeInt(1))})
	})
}

func TestPolygonRejectsMissingArgs(t *testing.T) {
	expectSVGPanic(t, func() { procPolygon(nil) })
}

func TestRawChecksArity(t *testing.T) {
	expectSVGPanic(t, func() { procRaw(nil) })
}
