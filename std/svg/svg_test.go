package svg

import (
	"strings"
	"testing"

	. "github.com/candid82/joker/core"
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
}
