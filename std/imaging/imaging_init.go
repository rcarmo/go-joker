package imaging

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var imagingNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.imaging"))

func init() {
	imagingNamespace.Lazy = initImagingNamespace
}

func initImagingNamespace() {
	imagingNamespace.ResetMeta(MakeMeta(nil, "Image processing: resize, crop, rotate, color adjustments, blur, sharpen, overlay. Pure Go, backed by disintegration/imaging.", "1.0"))

	procs := []struct {
		name string
		fn   ProcFn
		args string
		doc  string
	}{
		// I/O
		{"open", procOpen, "path", "Opens an image file. Supports PNG, JPEG, GIF, BMP, TIFF."},
		{"save", procSave, "img path", "Saves image to file. Format detected from extension."},
		{"encode", procEncode, "img format & quality", "Encodes image to bytes. Format: :png :jpeg :gif :bmp :tiff. Optional quality for JPEG."},
		{"decode", procDecode, "data", "Decodes image from byte string."},
		// Geometry
		{"resize", procResize, "img width height", "Resizes image to width×height. Use 0 for proportional."},
		{"fit", procFit, "img width height", "Resizes to fit within width×height preserving aspect ratio."},
		{"fill", procFill, "img width height & anchor", "Resizes and crops to fill exact width×height. Anchor: :center :top :bottom etc."},
		{"crop", procCrop, "img x y width height", "Crops region at x,y with given width and height."},
		{"crop-center", procCropCenter, "img width height", "Crops width×height from center of image."},
		{"rotate", procRotate, "img degrees", "Rotates image by degrees (any angle). Background is transparent."},
		{"flip-h", procFlipH, "img", "Flips image horizontally."},
		{"flip-v", procFlipV, "img", "Flips image vertically."},
		{"transpose", procTranspose, "img", "Transposes image (swap x and y axes)."},
		{"transverse", procTransverse, "img", "Transverses image (rotate 270° then flip)."},
		// Color
		{"grayscale", procGrayscale, "img", "Converts image to grayscale."},
		{"invert", procInvert, "img", "Inverts image colors."},
		{"brightness", procBrightness, "img amount", "Adjusts brightness. Range -100 to 100."},
		{"contrast", procContrast, "img amount", "Adjusts contrast. Range -100 to 100."},
		{"saturation", procSaturation, "img amount", "Adjusts saturation. Range -100 to 100."},
		{"gamma", procGamma, "img value", "Adjusts gamma correction. >1 brightens, <1 darkens."},
		{"sigmoid", procSigmoid, "img midpoint factor", "Applies sigmoid contrast. Midpoint 0-1, factor controls steepness."},
		// Effects
		{"blur", procBlur, "img sigma", "Applies gaussian blur with given sigma."},
		{"sharpen", procSharpen, "img sigma", "Sharpens image with given sigma."},
		// Compositing
		{"overlay", procOverlay, "base top x y & opacity", "Overlays top image at x,y with optional opacity (0.0-1.0)."},
		{"paste", procPaste, "base top x y", "Pastes image at x,y (no alpha blending)."},
		// Info
		{"width", procWidth, "img", "Returns image width in pixels."},
		{"height", procHeight, "img", "Returns image height in pixels."},
		{"bounds", procBounds, "img", "Returns [x y width height] of image bounds."},
		// Creation
		{"new", procNewImage, "width height & color", "Creates new image. Optional color as [r g b a] vector."},
	}

	for _, p := range procs {
		imagingNamespace.InternVar(p.name, Proc{Fn: p.fn, Name: "imaging/" + p.name},
			MakeMeta(
				NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, p.args))),
				p.doc, "1.0"))
	}
}
