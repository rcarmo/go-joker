package imaging

import (
	"math"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func TestNewAndInfo(t *testing.T) {
	initImagingNamespace()

	// Create a 100x50 red image
	color := corecollections.NewVectorFrom(coretypes.MakeInt(255), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))
	img := procNewImage([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(50), color})

	w := procWidth([]coretypes.Object{img})
	if w.(coretypes.Int).I != 100 {
		t.Fatalf("expected width 100, got %v", w)
	}

	h := procHeight([]coretypes.Object{img})
	if h.(coretypes.Int).I != 50 {
		t.Fatalf("expected height 50, got %v", h)
	}

	t.Logf("image: %s", img.ToString(false))
}

func TestPixelMutation(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]coretypes.Object{coretypes.MakeInt(3), coretypes.MakeInt(2), corecollections.NewVectorFrom(coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))})
	color := corecollections.NewVectorFrom(coretypes.MakeInt(12), coretypes.MakeInt(34), coretypes.MakeInt(56), coretypes.MakeInt(255))
	if got := procSetPixel([]coretypes.Object{img, coretypes.MakeInt(1), coretypes.MakeInt(0), color}); got != img {
		t.Fatal("set-pixel! should return the mutated image")
	}
	pixel := procPixel([]coretypes.Object{img, coretypes.MakeInt(1), coretypes.MakeInt(0)}).(coretypes.Indexed)
	for i, want := range []int{12, 34, 56, 255} {
		if got := pixel.Nth(i).(coretypes.Int).I; got != want {
			t.Fatalf("pixel[%d] = %d, want %d", i, got, want)
		}
	}
	assertImagingPanic(t, "pixel out of bounds", func() {
		procPixel([]coretypes.Object{img, coretypes.MakeInt(3), coretypes.MakeInt(0)})
	})
	assertImagingPanic(t, "set-pixel bad color", func() {
		procSetPixel([]coretypes.Object{img, coretypes.MakeInt(0), coretypes.MakeInt(0), corecollections.NewVectorFrom(coretypes.MakeInt(255))})
	})
	encoded := procEncode([]coretypes.Object{img, coretypes.MakeKeyword(STRINGS.Intern, "png")}).(coretypes.String)
	if len(encoded.S) == 0 {
		t.Fatal("encode :png returned empty data")
	}
}

func TestFromRGBA32(t *testing.T) {
	initImagingNamespace()

	pixels := corecollections.NewVectorFrom(
		coretypes.MakeInt(0xff0000ff),
		coretypes.MakeInt(0x00ff00ff),
		coretypes.MakeInt(0x0000ffff),
		coretypes.MakeInt(0x10203040),
	)
	img := procFromRGBA32([]coretypes.Object{coretypes.MakeInt(2), coretypes.MakeInt(2), pixels})

	for _, tt := range []struct {
		name     string
		x, y     int
		wantRGBA []int
	}{
		{name: "red", x: 0, y: 0, wantRGBA: []int{255, 0, 0, 255}},
		{name: "alpha", x: 1, y: 1, wantRGBA: []int{16, 32, 48, 64}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pixel := procPixel([]coretypes.Object{img, coretypes.MakeInt(tt.x), coretypes.MakeInt(tt.y)}).(coretypes.Indexed)
			for i, want := range tt.wantRGBA {
				if got := pixel.Nth(i).(coretypes.Int).I; got != want {
					t.Fatalf("pixel[%d] = %d, want %d", i, got, want)
				}
			}
		})
	}
	assertImagingPanic(t, "wrong packed pixel count", func() {
		procFromRGBA32([]coretypes.Object{coretypes.MakeInt(2), coretypes.MakeInt(2), corecollections.NewVectorFrom(coretypes.MakeInt(0))})
	})
	assertImagingPanic(t, "invalid packed pixel collection", func() {
		procFromRGBA32([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(1), coretypes.MakeString("x")})
	})
}

func TestResize(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]coretypes.Object{coretypes.MakeInt(200), coretypes.MakeInt(100), corecollections.NewVectorFrom(coretypes.MakeInt(0), coretypes.MakeInt(128), coretypes.MakeInt(255), coretypes.MakeInt(255))})
	resized := procResize([]coretypes.Object{img, coretypes.MakeInt(50), coretypes.MakeInt(25)})

	w := procWidth([]coretypes.Object{resized})
	h := procHeight([]coretypes.Object{resized})
	if w.(coretypes.Int).I != 50 || h.(coretypes.Int).I != 25 {
		t.Fatalf("expected 50x25, got %vx%v", w, h)
	}
	assertImagingPanic(t, "zero resize width", func() {
		procResize([]coretypes.Object{img, coretypes.MakeInt(0), coretypes.MakeInt(25)})
	})
}

func TestAdjustmentsRejectInvalidFloats(t *testing.T) {
	img := procNewImage([]coretypes.Object{coretypes.MakeInt(8), coretypes.MakeInt(8)})
	assertImagingPanic(t, "non-finite rotate angle", func() {
		procRotate([]coretypes.Object{img, coretypes.Double{D: math.Inf(1)}})
	})
	assertImagingPanic(t, "non-positive gamma", func() {
		procGamma([]coretypes.Object{img, coretypes.Double{D: 0}})
	})
	assertImagingPanic(t, "negative blur sigma", func() {
		procBlur([]coretypes.Object{img, coretypes.Double{D: -1}})
	})
	assertImagingPanic(t, "bad overlay opacity", func() {
		procOverlay([]coretypes.Object{img, img, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.Double{D: 2}})
	})
}

func TestGrayscaleAndBlur(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]coretypes.Object{coretypes.MakeInt(64), coretypes.MakeInt(64), corecollections.NewVectorFrom(coretypes.MakeInt(200), coretypes.MakeInt(100), coretypes.MakeInt(50), coretypes.MakeInt(255))})
	gray := procGrayscale([]coretypes.Object{img})
	blurred := procBlur([]coretypes.Object{gray, coretypes.Double{D: 2.0}})

	w := procWidth([]coretypes.Object{blurred})
	if w.(coretypes.Int).I != 64 {
		t.Fatal("size changed after grayscale+blur")
	}
}

func TestCropAndFlip(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]coretypes.Object{coretypes.MakeInt(100), coretypes.MakeInt(100), corecollections.NewVectorFrom(coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))})
	cropped := procCrop([]coretypes.Object{img, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(50), coretypes.MakeInt(30)})

	w := procWidth([]coretypes.Object{cropped})
	h := procHeight([]coretypes.Object{cropped})
	if w.(coretypes.Int).I != 50 || h.(coretypes.Int).I != 30 {
		t.Fatalf("crop: expected 50x30, got %vx%v", w, h)
	}

	flipped := procFlipH([]coretypes.Object{cropped})
	w2 := procWidth([]coretypes.Object{flipped})
	if w2.(coretypes.Int).I != 50 {
		t.Fatal("flip changed width")
	}
	assertImagingPanic(t, "negative crop width", func() {
		procCrop([]coretypes.Object{img, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(-1), coretypes.MakeInt(10)})
	})
}

func assertImagingPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s should panic", name)
		}
	}()
	f()
}

func TestNewImageRejectsInvalidInputs(t *testing.T) {
	assertImagingPanic(t, "short color vector", func() {
		procNewImage([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(1), corecollections.NewVectorFrom(coretypes.MakeInt(255))})
	})
	assertImagingPanic(t, "negative dimension", func() {
		procNewImage([]coretypes.Object{coretypes.MakeInt(-1), coretypes.MakeInt(1)})
	})
	assertImagingPanic(t, "color overflow", func() {
		procNewImage([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(1), corecollections.NewVectorFrom(coretypes.MakeInt(256), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))})
	})
}

func TestImagingInfoArityChecks(t *testing.T) {
	for name, proc := range map[string]ProcFn{
		"width":  procWidth,
		"height": procHeight,
		"bounds": procBounds,
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s should reject missing image", name)
				}
			}()
			proc(nil)
		})
	}
}
