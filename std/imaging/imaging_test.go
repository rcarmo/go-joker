package imaging

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestNewAndInfo(t *testing.T) {
	initImagingNamespace()

	// Create a 100x50 red image
	color := NewVectorFrom(MakeInt(255), MakeInt(0), MakeInt(0), MakeInt(255))
	img := procNewImage([]Object{MakeInt(100), MakeInt(50), color})

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

	img := procNewImage([]Object{MakeInt(200), MakeInt(100), NewVectorFrom(MakeInt(0), MakeInt(128), MakeInt(255), MakeInt(255))})
	resized := procResize([]Object{img, MakeInt(50), MakeInt(25)})

	w := procWidth([]Object{resized})
	h := procHeight([]Object{resized})
	if w.(Int).I != 50 || h.(Int).I != 25 {
		t.Fatalf("expected 50x25, got %vx%v", w, h)
	}
}

func TestGrayscaleAndBlur(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]Object{MakeInt(64), MakeInt(64), NewVectorFrom(MakeInt(200), MakeInt(100), MakeInt(50), MakeInt(255))})
	gray := procGrayscale([]Object{img})
	blurred := procBlur([]Object{gray, Double{D: 2.0}})

	w := procWidth([]Object{blurred})
	if w.(Int).I != 64 {
		t.Fatal("size changed after grayscale+blur")
	}
}

func TestCropAndFlip(t *testing.T) {
	initImagingNamespace()

	img := procNewImage([]Object{MakeInt(100), MakeInt(100), NewVectorFrom(MakeInt(0), MakeInt(0), MakeInt(0), MakeInt(255))})
	cropped := procCrop([]Object{img, MakeInt(10), MakeInt(10), MakeInt(50), MakeInt(30)})

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
		procNewImage([]Object{MakeInt(1), MakeInt(1), NewVectorFrom(MakeInt(255))})
	})
	assertImagingPanic(t, "negative dimension", func() {
		procNewImage([]Object{MakeInt(-1), MakeInt(1)})
	})
	assertImagingPanic(t, "color overflow", func() {
		procNewImage([]Object{MakeInt(1), MakeInt(1), NewVectorFrom(MakeInt(256), MakeInt(0), MakeInt(0), MakeInt(255))})
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
