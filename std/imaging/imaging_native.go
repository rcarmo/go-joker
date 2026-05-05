package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/disintegration/imaging"

	. "github.com/candid82/joker/core"
)

// Image wraps a Go image for use in Joker.
type Image struct {
	InfoHolder
	img *image.NRGBA
}

var typeImage = &Type{} // registered at init

func init() {
	typeImage = &Type{}
}

func (im *Image) ToString(escape bool) string {
	if im.img == nil {
		return "#<Image nil>"
	}
	b := im.img.Bounds()
	return fmt.Sprintf("#<Image %dx%d>", b.Dx(), b.Dy())
}

func (im *Image) Equals(other interface{}) bool {
	return im == other
}

func (im *Image) GetInfo() *ObjectInfo { return nil }
func (im *Image) WithInfo(info *ObjectInfo) Object { return im }
func (im *Image) GetType() *Type { return typeImage }
func (im *Image) Hash() uint32 { return 0 }

// --- Helpers ---

func extractImage(args []Object, idx int) *Image {
	img, ok := args[idx].(*Image)
	if !ok {
		panic(RT.NewError("Expected Image argument"))
	}
	return img
}

func wrapImage(img *image.NRGBA) *Image {
	return &Image{img: img}
}

func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	bounds := img.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x, y, img.At(x, y))
		}
	}
	return dst
}

func parseAnchor(s string) imaging.Anchor {
	switch s {
	case "center":
		return imaging.Center
	case "top-left":
		return imaging.TopLeft
	case "top":
		return imaging.Top
	case "top-right":
		return imaging.TopRight
	case "left":
		return imaging.Left
	case "right":
		return imaging.Right
	case "bottom-left":
		return imaging.BottomLeft
	case "bottom":
		return imaging.Bottom
	case "bottom-right":
		return imaging.BottomRight
	default:
		return imaging.Center
	}
}

// --- I/O ---

var procOpen ProcFn = func(args []Object) Object {
	path := ExtractString(args, 0)
	img, err := imaging.Open(path)
	if err != nil {
		panic(RT.NewError("imaging/open: " + err.Error()))
	}
	return wrapImage(toNRGBA(img))
}

var procSave ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	path := ExtractString(args, 1)
	err := imaging.Save(im.img, path)
	if err != nil {
		panic(RT.NewError("imaging/save: " + err.Error()))
	}
	return NIL
}

var procEncode ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	format := ExtractKeyword(args, 1)
	var buf bytes.Buffer
	switch format {
	case "png":
		err := png.Encode(&buf, im.img)
		if err != nil {
			panic(RT.NewError("imaging/encode: " + err.Error()))
		}
	case "jpeg", "jpg":
		quality := 90
		if len(args) > 2 {
			quality = ExtractInt(args, 2)
		}
		err := imaging.Encode(&buf, im.img, imaging.JPEG, imaging.JPEGQuality(quality))
		if err != nil {
			panic(RT.NewError("imaging/encode: " + err.Error()))
		}
	case "gif":
		err := imaging.Encode(&buf, im.img, imaging.GIF)
		if err != nil {
			panic(RT.NewError("imaging/encode: " + err.Error()))
		}
	case "bmp":
		err := imaging.Encode(&buf, im.img, imaging.BMP)
		if err != nil {
			panic(RT.NewError("imaging/encode: " + err.Error()))
		}
	case "tiff":
		err := imaging.Encode(&buf, im.img, imaging.TIFF)
		if err != nil {
			panic(RT.NewError("imaging/encode: " + err.Error()))
		}
	default:
		panic(RT.NewError("imaging/encode: unsupported format: " + format))
	}
	return MakeString(buf.String())
}

var procDecode ProcFn = func(args []Object) Object {
	data := ExtractString(args, 0)
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	if err != nil {
		panic(RT.NewError("imaging/decode: " + err.Error()))
	}
	return wrapImage(toNRGBA(img))
}

// --- Geometry ---

var procResize ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	w := ExtractInt(args, 1)
	h := ExtractInt(args, 2)
	return wrapImage(toNRGBA(imaging.Resize(im.img, w, h, imaging.Lanczos)))
}

var procFit ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	w := ExtractInt(args, 1)
	h := ExtractInt(args, 2)
	return wrapImage(toNRGBA(imaging.Fit(im.img, w, h, imaging.Lanczos)))
}

var procFill ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	w := ExtractInt(args, 1)
	h := ExtractInt(args, 2)
	anchor := "center"
	if len(args) > 3 {
		anchor = ExtractKeyword(args, 3)
	}
	return wrapImage(toNRGBA(imaging.Fill(im.img, w, h, parseAnchor(anchor), imaging.Lanczos)))
}

var procCrop ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	x := ExtractInt(args, 1)
	y := ExtractInt(args, 2)
	w := ExtractInt(args, 3)
	h := ExtractInt(args, 4)
	rect := image.Rect(x, y, x+w, y+h)
	return wrapImage(toNRGBA(imaging.Crop(im.img, rect)))
}

var procCropCenter ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	w := ExtractInt(args, 1)
	h := ExtractInt(args, 2)
	return wrapImage(toNRGBA(imaging.CropCenter(im.img, w, h)))
}

var procRotate ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	angle := ExtractDouble(args, 1)
	bg := color.NRGBA{0, 0, 0, 0}
	return wrapImage(toNRGBA(imaging.Rotate(im.img, angle, bg)))
}

var procFlipH ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.FlipH(im.img)))
}

var procFlipV ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.FlipV(im.img)))
}

var procTranspose ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Transpose(im.img)))
}

var procTransverse ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Transverse(im.img)))
}

// --- Color / Adjustment ---

var procGrayscale ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Grayscale(im.img)))
}

var procInvert ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Invert(im.img)))
}

var procBrightness ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	p := ExtractDouble(args, 1) // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustBrightness(im.img, p)))
}

var procContrast ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	p := ExtractDouble(args, 1) // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustContrast(im.img, p)))
}

var procSaturation ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	p := ExtractDouble(args, 1) // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustSaturation(im.img, p)))
}

var procGamma ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	g := ExtractDouble(args, 1)
	return wrapImage(toNRGBA(imaging.AdjustGamma(im.img, g)))
}

var procSigmoid ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	midpoint := ExtractDouble(args, 1)
	factor := ExtractDouble(args, 2)
	return wrapImage(toNRGBA(imaging.AdjustSigmoid(im.img, midpoint, factor)))
}

// --- Filters / Effects ---

var procBlur ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	sigma := ExtractDouble(args, 1)
	return wrapImage(toNRGBA(imaging.Blur(im.img, sigma)))
}

var procSharpen ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	sigma := ExtractDouble(args, 1)
	return wrapImage(toNRGBA(imaging.Sharpen(im.img, sigma)))
}

// --- Compositing ---

var procOverlay ProcFn = func(args []Object) Object {
	base := extractImage(args, 0)
	overlay := extractImage(args, 1)
	x := ExtractInt(args, 2)
	y := ExtractInt(args, 3)
	opacity := 1.0
	if len(args) > 4 {
		opacity = ExtractDouble(args, 4)
	}
	return wrapImage(toNRGBA(imaging.Overlay(base.img, overlay.img, image.Pt(x, y), opacity)))
}

var procPaste ProcFn = func(args []Object) Object {
	base := extractImage(args, 0)
	overlay := extractImage(args, 1)
	x := ExtractInt(args, 2)
	y := ExtractInt(args, 3)
	return wrapImage(toNRGBA(imaging.Paste(base.img, overlay.img, image.Pt(x, y))))
}

// --- Info ---

var procWidth ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return MakeInt(im.img.Bounds().Dx())
}

var procHeight ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	return MakeInt(im.img.Bounds().Dy())
}

var procBounds ProcFn = func(args []Object) Object {
	im := extractImage(args, 0)
	b := im.img.Bounds()
	return NewVectorFrom(
		MakeInt(b.Min.X),
		MakeInt(b.Min.Y),
		MakeInt(b.Dx()),
		MakeInt(b.Dy()),
	)
}

// --- New blank image ---

var procNewImage ProcFn = func(args []Object) Object {
	w := ExtractInt(args, 0)
	h := ExtractInt(args, 1)
	var c color.NRGBA
	if len(args) > 2 {
		v, ok := args[2].(Indexed)
		if !ok {
			panic(RT.NewError("imaging/new: color must be a vector [r g b a]"))
		}
		c.R = uint8(EnsureObjectIsInt(v.Nth(0), "").I)
		c.G = uint8(EnsureObjectIsInt(v.Nth(1), "").I)
		c.B = uint8(EnsureObjectIsInt(v.Nth(2), "").I)
		c.A = uint8(EnsureObjectIsInt(v.Nth(3), "").I)
	}
	return wrapImage(toNRGBA(imaging.New(w, h, c)))
}

// --- Registration ---

func init() {
	// Ensure image format decoders are registered
	_ = os.Getenv("") // force init
}
