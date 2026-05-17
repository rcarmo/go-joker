package imaging

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestNewAndInfo(t *testing.T) {
	initImagingNamespace()

	// Create a 100x50 red image
	color := NewVectorFrom(coretypes.MakeInt(255), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))
	img := procNewImage([]Object{coretypes.MakeInt(100), coretypes.MakeInt(50), color})

	w := procWidth([]Object{img})
	if w.(Int).I != 100 {
		t.Fatalf("expected width 100, got %v", w)
	}

	h := procHeight([]Object{img})
	if h.(Int).I != 50 {
		t.Fatalf("expected height 50, got %v", h)
	}

	t.Logf("image: %s", img.ToString(false))
}

func TestResize(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]Object{coretypes.MakeInt(200), coretypes.MakeInt(100), NewVectorFrom(coretypes.MakeInt(0), coretypes.MakeInt(128), coretypes.MakeInt(255), coretypes.MakeInt(255))})
	resized := procResize([]Object{img, coretypes.MakeInt(50), coretypes.MakeInt(25)})

	w := procWidth([]Object{resized})
	h := procHeight([]Object{resized})
	if w.(Int).I != 50 || h.(Int).I != 25 {
		t.Fatalf("expected 50x25, got %vx%v", w, h)
	}
	assertImagingPanic(t, "zero resize width", func() {
		procResize([]Object{img, coretypes.MakeInt(0), coretypes.MakeInt(25)})
	})
}

func TestAdjustmentsRejectInvalidFloats(t *testing.T) {
	img := procNewImage([]Object{coretypes.MakeInt(8), coretypes.MakeInt(8)})
	assertImagingPanic(t, "non-finite rotate angle", func() {
		procRotate([]Object{img, Double{D: math.Inf(1)}})
	})
	assertImagingPanic(t, "non-positive gamma", func() {
		procGamma([]Object{img, Double{D: 0}})
	})
	assertImagingPanic(t, "negative blur sigma", func() {
		procBlur([]Object{img, Double{D: -1}})
	})
	assertImagingPanic(t, "bad overlay opacity", func() {
		procOverlay([]Object{img, img, coretypes.MakeInt(0), coretypes.MakeInt(0), Double{D: 2}})
	})
}

func TestGrayscaleAndBlur(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]Object{coretypes.MakeInt(64), coretypes.MakeInt(64), NewVectorFrom(coretypes.MakeInt(200), coretypes.MakeInt(100), coretypes.MakeInt(50), coretypes.MakeInt(255))})
	gray := procGrayscale([]Object{img})
	blurred := procBlur([]Object{gray, Double{D: 2.0}})

	w := procWidth([]Object{blurred})
	if w.(Int).I != 64 {
		t.Fatal("size changed after grayscale+blur")
	}
}

func TestCropAndFlip(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]Object{coretypes.MakeInt(100), coretypes.MakeInt(100), NewVectorFrom(coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))})
	cropped := procCrop([]Object{img, coretypes.MakeInt(10), coretypes.MakeInt(10), coretypes.MakeInt(50), coretypes.MakeInt(30)})

	w := procWidth([]Object{cropped})
	h := procHeight([]Object{cropped})
	if w.(Int).I != 50 || h.(Int).I != 30 {
		t.Fatalf("crop: expected 50x30, got %vx%v", w, h)
	}

	flipped := procFlipH([]Object{cropped})
	w2 := procWidth([]Object{flipped})
	if w2.(Int).I != 50 {
		t.Fatal("flip changed width")
	}
	assertImagingPanic(t, "negative crop width", func() {
		procCrop([]Object{img, coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(-1), coretypes.MakeInt(10)})
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
		procNewImage([]Object{coretypes.MakeInt(1), coretypes.MakeInt(1), NewVectorFrom(coretypes.MakeInt(255))})
	})
	assertImagingPanic(t, "negative dimension", func() {
		procNewImage([]Object{coretypes.MakeInt(-1), coretypes.MakeInt(1)})
	})
	assertImagingPanic(t, "color overflow", func() {
		procNewImage([]Object{coretypes.MakeInt(1), coretypes.MakeInt(1), NewVectorFrom(coretypes.MakeInt(256), coretypes.MakeInt(0), coretypes.MakeInt(0), coretypes.MakeInt(255))})
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
