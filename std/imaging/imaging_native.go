package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	"github.com/disintegration/imaging"

	. "github.com/rcarmo/go-joker/core"
)

// Image wraps a Go image for use in Joker.
type Image struct {
	coretypes.InfoHolder
	img *image.NRGBA
}

var typeImage = &coretypes.Type{} // registered at init

func init() {
	typeImage = &coretypes.Type{}
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

func (im *Image) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (im *Image) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return im }
func (im *Image) GetType() *coretypes.Type                             { return typeImage }
func (im *Image) Hash() uint32                                         { return 0 }

// --- Helpers ---

func extractImage(args []coretypes.Object, idx int) *Image {
	if idx < 0 || idx >= len(args) {
		panic(RT.NewError("Expected Image argument"))
	}
	img, ok := args[idx].(*Image)
	if !ok {
		panic(RT.NewError("Expected Image argument"))
	}
	return img
}

func wrapImage(img *image.NRGBA) *Image {
	return &Image{img: img}
}

// WrapImage creates a Joker Image from an *image.NRGBA (exported for other packages).
func WrapImage(img *image.NRGBA) coretypes.Object {
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
	s = strings.TrimPrefix(s, ":")
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

var procOpen ProcFn = func(args []coretypes.Object) coretypes.Object {
	path := coretypes.ExtractString(args, 0)
	img, err := imaging.Open(path)
	if err != nil {
		panic(RT.NewError("imaging/open: " + err.Error()))
	}
	return wrapImage(toNRGBA(img))
}

var procSave ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	path := coretypes.ExtractString(args, 1)
	err := imaging.Save(im.img, path)
	if err != nil {
		panic(RT.NewError("imaging/save: " + err.Error()))
	}
	return NIL
}

var procEncode ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	format := strings.TrimPrefix(coretypes.ExtractKeyword(args, 1), ":")
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
			quality = coretypes.ExtractInt(args, 2)
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
	return coretypes.MakeString(buf.String())
}

var procDecode ProcFn = func(args []coretypes.Object) coretypes.Object {
	data := coretypes.ExtractString(args, 0)
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	if err != nil {
		panic(RT.NewError("imaging/decode: " + err.Error()))
	}
	return wrapImage(toNRGBA(img))
}

// --- Geometry ---

func positiveDimension(obj coretypes.Object, name string) int {
	v := coretypes.EnsureObjectIsInt(obj, "imaging "+name+": %s").I
	if v <= 0 {
		panic(RT.NewError("imaging: " + name + " must be positive"))
	}
	return v
}

func finiteFloat(obj coretypes.Object, name string) float64 {
	v := coretypes.ExtractDouble([]coretypes.Object{obj}, 0)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		panic(RT.NewError("imaging: " + name + " must be finite"))
	}
	return v
}

func nonNegativeFloat(obj coretypes.Object, name string) float64 {
	v := finiteFloat(obj, name)
	if v < 0 {
		panic(RT.NewError("imaging: " + name + " must be non-negative"))
	}
	return v
}

func positiveFloat(obj coretypes.Object, name string) float64 {
	v := finiteFloat(obj, name)
	if v <= 0 {
		panic(RT.NewError("imaging: " + name + " must be positive"))
	}
	return v
}

func opacityFloat(obj coretypes.Object) float64 {
	v := finiteFloat(obj, "opacity")
	if v < 0 || v > 1 {
		panic(RT.NewError("imaging: opacity must be in [0,1]"))
	}
	return v
}

var procResize ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	w := positiveDimension(args[1], "width")
	h := positiveDimension(args[2], "height")
	return wrapImage(toNRGBA(imaging.Resize(im.img, w, h, imaging.Lanczos)))
}

var procFit ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	w := positiveDimension(args[1], "width")
	h := positiveDimension(args[2], "height")
	return wrapImage(toNRGBA(imaging.Fit(im.img, w, h, imaging.Lanczos)))
}

var procFill ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	w := positiveDimension(args[1], "width")
	h := positiveDimension(args[2], "height")
	anchor := "center"
	if len(args) > 3 {
		anchor = coretypes.ExtractKeyword(args, 3)
	}
	return wrapImage(toNRGBA(imaging.Fill(im.img, w, h, parseAnchor(anchor), imaging.Lanczos)))
}

var procCrop ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	x := coretypes.ExtractInt(args, 1)
	y := coretypes.ExtractInt(args, 2)
	w := positiveDimension(args[3], "width")
	h := positiveDimension(args[4], "height")
	rect := image.Rect(x, y, x+w, y+h)
	return wrapImage(toNRGBA(imaging.Crop(im.img, rect)))
}

var procCropCenter ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	w := positiveDimension(args[1], "width")
	h := positiveDimension(args[2], "height")
	return wrapImage(toNRGBA(imaging.CropCenter(im.img, w, h)))
}

var procRotate ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	angle := finiteFloat(args[1], "angle")
	bg := color.NRGBA{0, 0, 0, 0}
	return wrapImage(toNRGBA(imaging.Rotate(im.img, angle, bg)))
}

var procFlipH ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.FlipH(im.img)))
}

var procFlipV ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.FlipV(im.img)))
}

var procTranspose ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Transpose(im.img)))
}

var procTransverse ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Transverse(im.img)))
}

// --- Color / Adjustment ---

var procGrayscale ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Grayscale(im.img)))
}

var procInvert ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	return wrapImage(toNRGBA(imaging.Invert(im.img)))
}

var procBrightness ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	p := finiteFloat(args[1], "brightness") // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustBrightness(im.img, p)))
}

var procContrast ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	p := finiteFloat(args[1], "contrast") // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustContrast(im.img, p)))
}

var procSaturation ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	p := finiteFloat(args[1], "saturation") // -100 to 100
	return wrapImage(toNRGBA(imaging.AdjustSaturation(im.img, p)))
}

var procGamma ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	g := positiveFloat(args[1], "gamma")
	return wrapImage(toNRGBA(imaging.AdjustGamma(im.img, g)))
}

var procSigmoid ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	midpoint := finiteFloat(args[1], "midpoint")
	factor := finiteFloat(args[2], "factor")
	return wrapImage(toNRGBA(imaging.AdjustSigmoid(im.img, midpoint, factor)))
}

// --- Filters / Effects ---

var procBlur ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	sigma := nonNegativeFloat(args[1], "sigma")
	return wrapImage(toNRGBA(imaging.Blur(im.img, sigma)))
}

var procSharpen ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	sigma := nonNegativeFloat(args[1], "sigma")
	return wrapImage(toNRGBA(imaging.Sharpen(im.img, sigma)))
}

// --- Compositing ---

var procOverlay ProcFn = func(args []coretypes.Object) coretypes.Object {
	base := extractImage(args, 0)
	overlay := extractImage(args, 1)
	x := coretypes.ExtractInt(args, 2)
	y := coretypes.ExtractInt(args, 3)
	opacity := 1.0
	if len(args) > 4 {
		opacity = opacityFloat(args[4])
	}
	return wrapImage(toNRGBA(imaging.Overlay(base.img, overlay.img, image.Pt(x, y), opacity)))
}

var procPaste ProcFn = func(args []coretypes.Object) coretypes.Object {
	base := extractImage(args, 0)
	overlay := extractImage(args, 1)
	x := coretypes.ExtractInt(args, 2)
	y := coretypes.ExtractInt(args, 3)
	return wrapImage(toNRGBA(imaging.Paste(base.img, overlay.img, image.Pt(x, y))))
}

// --- Info ---

var procWidth ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	im := extractImage(args, 0)
	return coretypes.MakeInt(im.img.Bounds().Dx())
}

var procHeight ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	im := extractImage(args, 0)
	return coretypes.MakeInt(im.img.Bounds().Dy())
}

var procBounds ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	im := extractImage(args, 0)
	b := im.img.Bounds()
	return corecollections.NewVectorFrom(
		coretypes.MakeInt(b.Min.X),
		coretypes.MakeInt(b.Min.Y),
		coretypes.MakeInt(b.Dx()),
		coretypes.MakeInt(b.Dy()),
	)
}

// --- New blank image ---

func colorChannel(obj coretypes.Object, name string) uint8 {
	v := coretypes.EnsureObjectIsInt(obj, "imaging/new color "+name+": %s").I
	if v < 0 || v > 255 {
		panic(RT.NewError("imaging/new: color channel " + name + " must be in [0,255]"))
	}
	return uint8(v)
}

func imageColor(obj coretypes.Object, op string) color.NRGBA {
	v, ok := obj.(coretypes.Indexed)
	counted, countedOk := obj.(coretypes.Counted)
	if !ok || !countedOk || counted.Count() != 4 {
		panic(RT.NewError(op + ": color must be a vector [r g b a]"))
	}
	channel := func(idx int, name string) uint8 {
		n := coretypes.EnsureObjectIsInt(v.Nth(idx), op+" color "+name+": %s").I
		if n < 0 || n > 255 {
			panic(RT.NewError(op + ": color channel " + name + " must be in [0,255]"))
		}
		return uint8(n)
	}
	return color.NRGBA{
		R: channel(0, "r"),
		G: channel(1, "g"),
		B: channel(2, "b"),
		A: channel(3, "a"),
	}
}

var procNewImage ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 3)
	w := coretypes.ExtractInt(args, 0)
	h := coretypes.ExtractInt(args, 1)
	if w < 0 || h < 0 {
		panic(RT.NewError("imaging/new: dimensions must be non-negative"))
	}
	var c color.NRGBA
	if len(args) > 2 {
		v, ok := args[2].(coretypes.Indexed)
		counted, countedOk := args[2].(coretypes.Counted)
		if !ok || !countedOk || counted.Count() != 4 {
			panic(RT.NewError("imaging/new: color must be a vector [r g b a]"))
		}
		c.R = colorChannel(v.Nth(0), "r")
		c.G = colorChannel(v.Nth(1), "g")
		c.B = colorChannel(v.Nth(2), "b")
		c.A = colorChannel(v.Nth(3), "a")
	}
	return wrapImage(toNRGBA(imaging.New(w, h, c)))
}

func pixelPoint(im *Image, x, y int, op string) {
	bounds := im.img.Bounds()
	if x < bounds.Min.X || x >= bounds.Max.X || y < bounds.Min.Y || y >= bounds.Max.Y {
		panic(RT.NewError(fmt.Sprintf("%s: pixel coordinate out of bounds: %d,%d", op, x, y)))
	}
}

var procPixel ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 3)
	im := extractImage(args, 0)
	x := coretypes.ExtractInt(args, 1)
	y := coretypes.ExtractInt(args, 2)
	pixelPoint(im, x, y, "imaging/pixel")
	c := im.img.NRGBAAt(x, y)
	return corecollections.NewVectorFrom(
		coretypes.MakeInt(int(c.R)),
		coretypes.MakeInt(int(c.G)),
		coretypes.MakeInt(int(c.B)),
		coretypes.MakeInt(int(c.A)),
	)
}

var procSetPixel ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 4, 4)
	im := extractImage(args, 0)
	x := coretypes.ExtractInt(args, 1)
	y := coretypes.ExtractInt(args, 2)
	pixelPoint(im, x, y, "imaging/set-pixel!")
	im.img.SetNRGBA(x, y, imageColor(args[3], "imaging/set-pixel!"))
	return im
}

// --- Registration ---
