package imaging

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	_ "golang.org/x/image/webp"

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

func encodeImage(im *Image, format string, quality int) []byte {
	format = strings.TrimPrefix(format, ":")
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, im.img)
	case "jpeg", "jpg":
		err = imaging.Encode(&buf, im.img, imaging.JPEG, imaging.JPEGQuality(quality))
	case "gif":
		err = imaging.Encode(&buf, im.img, imaging.GIF)
	case "bmp":
		err = imaging.Encode(&buf, im.img, imaging.BMP)
	case "tiff":
		err = imaging.Encode(&buf, im.img, imaging.TIFF)
	default:
		panic(RT.NewError("imaging/encode: unsupported format: " + format))
	}
	if err != nil {
		panic(RT.NewError("imaging/encode: " + err.Error()))
	}
	return buf.Bytes()
}

func encodeArgs(args []coretypes.Object) (*Image, string, int) {
	im := extractImage(args, 0)
	format := strings.TrimPrefix(coretypes.ExtractKeyword(args, 1), ":")
	quality := 90
	if len(args) > 2 {
		quality = coretypes.ExtractInt(args, 2)
	}
	return im, format, quality
}

var procEncode ProcFn = func(args []coretypes.Object) coretypes.Object {
	im, format, quality := encodeArgs(args)
	return coretypes.MakeString(string(encodeImage(im, format, quality)))
}

var procBytes ProcFn = func(args []coretypes.Object) coretypes.Object {
	im, format, quality := encodeArgs(args)
	return coretypes.MakeString(string(encodeImage(im, format, quality)))
}

var procBase64 ProcFn = func(args []coretypes.Object) coretypes.Object {
	im, format, quality := encodeArgs(args)
	return coretypes.MakeString(base64.StdEncoding.EncodeToString(encodeImage(im, format, quality)))
}

var procDataURI ProcFn = func(args []coretypes.Object) coretypes.Object {
	im, format, quality := encodeArgs(args)
	mime := "image/" + format
	if format == "jpg" {
		mime = "image/jpeg"
	}
	return coretypes.MakeString("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(encodeImage(im, format, quality)))
}

// EncodePNG encodes a Joker Image as PNG bytes for host-side renderers.
func EncodePNG(obj coretypes.Object) ([]byte, bool, error) {
	im, ok := obj.(*Image)
	if !ok {
		return nil, false, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im.img); err != nil {
		return nil, true, err
	}
	return buf.Bytes(), true, nil
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

func hashHex(bits []bool) string {
	out := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		if bit {
			out[i/8] |= 1 << uint(7-(i%8))
		}
	}
	return hex.EncodeToString(out)
}

func gray8(c color.NRGBA) uint16 {
	return uint16(c.R)*299 + uint16(c.G)*587 + uint16(c.B)*114
}

var procAverageHash ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	small := imaging.Resize(im.img, 8, 8, imaging.Lanczos)
	vals := make([]uint16, 0, 64)
	var sum uint32
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			v := gray8(small.NRGBAAt(x, y))
			vals = append(vals, v)
			sum += uint32(v)
		}
	}
	avg := uint16(sum / 64)
	bits := make([]bool, 64)
	for i, v := range vals {
		bits[i] = v >= avg
	}
	return coretypes.MakeString(hashHex(bits))
}

var procDifferenceHash ProcFn = func(args []coretypes.Object) coretypes.Object {
	im := extractImage(args, 0)
	small := imaging.Resize(im.img, 9, 8, imaging.Lanczos)
	bits := make([]bool, 0, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			bits = append(bits, gray8(small.NRGBAAt(x, y)) > gray8(small.NRGBAAt(x+1, y)))
		}
	}
	return coretypes.MakeString(hashHex(bits))
}

var procImageHash ProcFn = func(args []coretypes.Object) coretypes.Object { return procDifferenceHash(args) }

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

var procMetadata ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	im := extractImage(args, 0)
	b := im.img.Bounds()
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "width"), coretypes.MakeInt(b.Dx()))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "height"), coretypes.MakeInt(b.Dy()))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "bounds"), corecollections.NewVectorFrom(
		coretypes.MakeInt(b.Min.X),
		coretypes.MakeInt(b.Min.Y),
		coretypes.MakeInt(b.Dx()),
		coretypes.MakeInt(b.Dy()),
	))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "color-model"), coretypes.MakeKeyword(STRINGS.Intern, "nrgba"))
	return m
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

func packedRGBA32(obj coretypes.Object, index int) uint32 {
	n := coretypes.EnsureObjectIsNumber(obj, fmt.Sprintf("imaging/from-rgba32: pixel %d must be an integer", index))
	return uint32(n.Int().I)
}

var procFromRGBA32 ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 3)
	w := positiveDimension(args[0], "width")
	h := positiveDimension(args[1], "height")
	pixels, ok := args[2].(coretypes.CountedIndexed)
	if !ok {
		panic(RT.NewError("imaging/from-rgba32: pixels must be an indexed collection"))
	}
	expected := w * h
	if pixels.Count() != expected {
		panic(RT.NewError(fmt.Sprintf("imaging/from-rgba32: expected %d pixels, got %d", expected, pixels.Count())))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < expected; i++ {
		rgba := packedRGBA32(pixels.At(i), i)
		off := i * 4
		img.Pix[off] = uint8(rgba >> 24)
		img.Pix[off+1] = uint8(rgba >> 16)
		img.Pix[off+2] = uint8(rgba >> 8)
		img.Pix[off+3] = uint8(rgba)
	}
	return wrapImage(img)
}

var procFromRGBA32Fn ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 3)
	w := positiveDimension(args[0], "width")
	h := positiveDimension(args[1], "height")
	pixelFn, ok := args[2].(coretypes.Callable)
	if !ok {
		panic(RT.NewError("imaging/from-rgba32-fn: pixel function must be callable"))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	callArgs := []coretypes.Object{coretypes.Int{}, coretypes.Int{}}
	i := 0
	for y := 0; y < h; y++ {
		callArgs[1] = coretypes.MakeInt(y)
		for x := 0; x < w; x++ {
			callArgs[0] = coretypes.MakeInt(x)
			rgba := packedRGBA32(pixelFn.Call(callArgs), i)
			off := i * 4
			img.Pix[off] = uint8(rgba >> 24)
			img.Pix[off+1] = uint8(rgba >> 16)
			img.Pix[off+2] = uint8(rgba >> 8)
			img.Pix[off+3] = uint8(rgba)
			i++
		}
	}
	return wrapImage(img)
}

var procFromRGBA32DomainFn ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 7, 7)
	w := positiveDimension(args[0], "width")
	h := positiveDimension(args[1], "height")
	xmin := coretypes.EnsureArgIsNumber(args, 2).Double().D
	ymin := coretypes.EnsureArgIsNumber(args, 3).Double().D
	dx := coretypes.EnsureArgIsNumber(args, 4).Double().D
	dy := coretypes.EnsureArgIsNumber(args, 5).Double().D
	pixelFn, ok := args[6].(coretypes.Callable)
	if !ok {
		panic(RT.NewError("imaging/from-rgba32-domain-fn: pixel function must be callable"))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	callArgs := []coretypes.Object{coretypes.Double{}, coretypes.Double{}}
	i := 0
	for y := 0; y < h; y++ {
		callArgs[1] = coretypes.Double{D: ymin + dy*float64(y)}
		for x := 0; x < w; x++ {
			callArgs[0] = coretypes.Double{D: xmin + dx*float64(x)}
			rgba := packedRGBA32(pixelFn.Call(callArgs), i)
			off := i * 4
			img.Pix[off] = uint8(rgba >> 24)
			img.Pix[off+1] = uint8(rgba >> 16)
			img.Pix[off+2] = uint8(rgba >> 8)
			img.Pix[off+3] = uint8(rgba)
			i++
		}
	}
	return wrapImage(img)
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

// --- Fractal flame (native, high-performance) ---

var procFractalFlame ProcFn = func(args []coretypes.Object) coretypes.Object {
	// (fractal-flame width height opts)
	// opts is a map with:
	//   :iterations - number of chaos game iterations (default 5000000)
	//   :transforms - vector of transform maps
	//   :palette    - keyword (:fire :ice :plasma :green, default :fire)
	//   :gamma      - gamma correction (default 0.6)
	//   :xmin/:xmax/:ymin/:ymax - view bounds (default -2..2)
	CheckArity(args, 2, 3)
	width := coretypes.ExtractInt(args, 0)
	height := coretypes.ExtractInt(args, 1)
	if width < 1 || height < 1 || width > 8192 || height > 8192 {
		panic(RT.NewError("imaging/fractal-flame: dimensions must be 1-8192"))
	}

	// Parse options
	iters := 5000000
	gamma := 0.6
	xmin, xmax := -2.0, 2.0
	ymin, ymax := -2.0, 2.0
	paletteKey := "fire"

	type xform struct {
		a, b, c, d, e, f float64
		variation        int // 0=linear, 1=spherical, 2=swirl, 3=horseshoe, 4=diamond
		weight, color    float64
	}

	// Default flame system (produces nice output)
	xforms := []xform{
		{-0.681206, -0.0779465, 0.20769, 0.0779465, -0.681206, 0.15589, 2, 0.50, 0.0},
		{0.953766, 0.48187, 0.43268, -0.48187, 0.953766, 0.0413, 0, 0.25, 0.5},
		{0.5613, -0.3254, -0.4827, 0.3254, 0.5613, 0.2836, 1, 0.15, 0.85},
		{-0.1632, 0.7124, 0.0512, -0.7124, -0.1632, 0.2051, 3, 0.07, 0.35},
		{0.3731, -0.6421, 0.1523, 0.6421, 0.3731, -0.0821, 4, 0.03, 0.7},
	}

	if len(args) > 2 && args[2] != nil && !args[2].Equals(NIL) {
		opts, ok := args[2].(coretypes.Map)
		if !ok {
			panic(RT.NewError("imaging/fractal-flame: third arg must be a map"))
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "iterations")); found {
			iters = v.(coretypes.Int).I
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "gamma")); found {
			gamma = extractFloat(v)
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "xmin")); found {
			xmin = extractFloat(v)
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "xmax")); found {
			xmax = extractFloat(v)
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "ymin")); found {
			ymin = extractFloat(v)
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "ymax")); found {
			ymax = extractFloat(v)
		}
		if found, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "palette")); found {
			if kw, ok := v.(coretypes.Keyword); ok {
				paletteKey = kw.Name()
			}
		}
	}

	// Build cumulative weight table
	cumWeights := make([]float64, len(xforms))
	sum := 0.0
	for i, xf := range xforms {
		sum += xf.weight
		cumWeights[i] = sum
	}
	// Normalize
	for i := range cumWeights {
		cumWeights[i] /= sum
	}

	// Histogram
	npixels := width * height
	counts := make([]uint32, npixels)
	colorSums := make([]float64, npixels)

	// Chaos game iteration
	rng := newXorShift(42)
	x, y, c := rng.float64()*4.0-2.0, rng.float64()*4.0-2.0, 0.5

	scaleX := float64(width) / (xmax - xmin)
	scaleY := float64(height) / (ymax - ymin)

	for iter := 0; iter < iters; iter++ {
		// Select transform
		r := rng.float64()
		var xf xform
		for j, cw := range cumWeights {
			if r < cw {
				xf = xforms[j]
				break
			}
		}

		// Affine
		ax := xf.a*x + xf.b*y + xf.c
		ay := xf.d*x + xf.e*y + xf.f

		// Variation
		switch xf.variation {
		case 0: // linear
			x, y = ax, ay
		case 1: // spherical
			r2 := ax*ax + ay*ay
			if r2 < 1e-10 {
				r2 = 1e-10
			}
			x, y = ax/r2, ay/r2
		case 2: // swirl
			r2 := ax*ax + ay*ay
			s := math.Sin(r2)
			cos := math.Cos(r2)
			x = ax*s - ay*cos
			y = ax*cos + ay*s
		case 3: // horseshoe
			r2 := ax*ax + ay*ay
			if r2 < 1e-10 {
				r2 = 1e-10
			}
			rr := math.Sqrt(r2)
			x = (ax - ay) * (ax + ay) / rr
			y = 2.0 * ax * ay / rr
		case 4: // diamond
			r2 := ax*ax + ay*ay
			if r2 < 1e-10 {
				r2 = 1e-10
			}
			rr := math.Sqrt(r2)
			theta := math.Atan2(ay, ax)
			x = math.Sin(theta) * rr
			y = math.Cos(theta) / rr
		}

		// Color blend
		c = (c + xf.color) / 2.0

		// Map to pixel and accumulate (skip warmup)
		if iter > 20 {
			px := int((x - xmin) * scaleX)
			py := int((y - ymin) * scaleY)
			if px >= 0 && px < width && py >= 0 && py < height {
				idx := py*width + px
				counts[idx]++
				colorSums[idx] += c
			}
		}
	}

	// Tone mapping: log-density
	var maxCount uint32
	for _, cnt := range counts {
		if cnt > maxCount {
			maxCount = cnt
		}
	}
	logMax := math.Log(1.0 + float64(maxCount))

	// Render to image (black background)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	// Fill with opaque black
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	for idx := 0; idx < npixels; idx++ {
		cnt := counts[idx]
		if cnt == 0 {
			continue
		}
		alpha := math.Pow(math.Log(1.0+float64(cnt))/logMax, gamma)
		avgColor := colorSums[idx] / float64(cnt)

		// Palette
		var pr, pg, pb float64
		switch paletteKey {
		case "ice":
			pr = avgColor * 0.3
			pg = avgColor * 0.7
			pb = 0.5 + avgColor*0.5
		case "plasma":
			pr = 0.5 + 0.5*math.Sin(avgColor*6.28)
			pg = 0.5 + 0.5*math.Sin(avgColor*6.28+2.09)
			pb = 0.5 + 0.5*math.Sin(avgColor*6.28+4.19)
		case "green":
			pr = avgColor * 0.2
			pg = 0.3 + avgColor*0.7
			pb = avgColor * 0.3
		default: // fire
			pr = math.Min(1.0, avgColor*3.0)
			pg = math.Max(0.0, math.Min(1.0, avgColor*3.0-1.0))
			pb = math.Max(0.0, math.Min(1.0, avgColor*3.0-2.0))
		}

		xx := idx % width
		yy := idx / width
		img.SetNRGBA(xx, yy, color.NRGBA{
			R: uint8(alpha * pr * 255),
			G: uint8(alpha * pg * 255),
			B: uint8(alpha * pb * 255),
			A: uint8(math.Min(255, alpha*255)),
		})
	}

	return wrapImage(img)
}

// xorShift64 PRNG for fast deterministic random in flame iteration
type xorShift64 struct {
	s uint64
}

func newXorShift(seed uint64) *xorShift64 {
	if seed == 0 {
		seed = 0xdeadbeefcafe1234
	}
	return &xorShift64{s: seed}
}

func (x *xorShift64) next() uint64 {
	x.s ^= x.s << 13
	x.s ^= x.s >> 7
	x.s ^= x.s << 17
	return x.s
}

func (x *xorShift64) float64() float64 {
	return float64(x.next()>>11) / (1 << 53)
}

func extractFloat(o coretypes.Object) float64 {
	switch v := o.(type) {
	case coretypes.Int:
		return float64(v.I)
	case coretypes.Double:
		return v.D
	default:
		panic(RT.NewError("expected number"))
	}
}

// --- Registration ---
