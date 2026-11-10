package preview

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf16"
	"unsafe"

	"fontlens/internal/fonts"
	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d/common"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/directwrite"
	dxgicommon "github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dxgi/common"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/imaging"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
)

var renderSlots = make(chan struct{}, 2)

const maxPreviewBitmapBytes = 8 << 20

// A zero width renders a complete single line at its natural width.
// A zero height with a positive width fits the wrapped text's height.
func Render(ctx context.Context, path string, faceIndex int, axes map[string]float32, sample string, size, lineHeight float32, width, height int, scale float32, ink color.RGBA) (*image.RGBA, error) {
	if width < 0 || height < 0 || width == 0 && height == 0 || size <= 0 || scale <= 0 {
		return nil, fmt.Errorf("invalid preview dimensions")
	}
	if strings.ContainsRune(sample, 0) {
		return nil, fmt.Errorf("预览文字不能包含空字符")
	}
	select {
	case renderSlots <- struct{}{}:
		defer func() { <-renderSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := com.CoInitializeEx(uint32(com.COINIT_MULTITHREADED)); err != nil {
		return nil, err
	}
	defer com.CoUninitialize()
	var unknown *win32.IUnknown
	if err := directwrite.DWriteCreateFactory(directwrite.DWRITE_FACTORY_TYPE_ISOLATED, &directwrite.IID_IDWriteFactory, &unknown); err != nil {
		return nil, err
	}
	factory := (*directwrite.IDWriteFactory)(unsafe.Pointer(unknown))
	defer factory.Release()
	factory3, err := win32.QueryInterface[directwrite.IDWriteFactory3](factory, &directwrite.IID_IDWriteFactory3)
	if err != nil {
		return nil, err
	}
	defer factory3.Release()
	var file *directwrite.IDWriteFontFile
	var sfnt []byte
	if strings.EqualFold(filepath.Ext(path), ".woff") || strings.EqualFold(filepath.Ext(path), ".woff2") {
		sfnt, err = fonts.DecodeWebFont(path)
		if err != nil {
			return nil, err
		}
		defer runtime.KeepAlive(sfnt)
		factory5, err := win32.QueryInterface[directwrite.IDWriteFactory5](factory, &directwrite.IID_IDWriteFactory5)
		if err != nil {
			return nil, err
		}
		defer factory5.Release()
		var loader *directwrite.IDWriteInMemoryFontFileLoader
		if err := factory5.CreateInMemoryFontFileLoader(&loader); err != nil {
			return nil, err
		}
		defer loader.Release()
		if err := factory.RegisterFontFileLoader(&loader.IDWriteFontFileLoader); err != nil {
			return nil, err
		}
		defer factory.UnregisterFontFileLoader(&loader.IDWriteFontFileLoader)
		if err := loader.CreateInMemoryFontFileReference(factory, sfnt, nil, &file); err != nil {
			return nil, err
		}
	} else if err := factory.CreateFontFileReference(path, nil, &file); err != nil {
		return nil, err
	}
	defer file.Release()
	var supported foundation.BOOL
	var fileType directwrite.DWRITE_FONT_FILE_TYPE
	var faceType directwrite.DWRITE_FONT_FACE_TYPE
	var count uint32
	if err := file.Analyze(&supported, &fileType, &faceType, &count); err != nil {
		return nil, err
	}
	if supported == 0 || faceIndex < 0 || uint32(faceIndex) >= count {
		return nil, fmt.Errorf("DirectWrite cannot open face %d", faceIndex)
	}
	var reference *directwrite.IDWriteFontFaceReference
	if err := factory3.CreateFontFaceReference(file, uint32(faceIndex), directwrite.DWRITE_FONT_SIMULATIONS_NONE, &reference); err != nil {
		return nil, err
	}
	defer reference.Release()
	var face *directwrite.IDWriteFontFace3
	if err := reference.CreateFontFace(&face); err != nil {
		return nil, err
	}
	defer face.Release()
	codepoints := make([]uint32, 0, len(sample))
	textRanges := make([]directwrite.DWRITE_TEXT_RANGE, 0, len(sample))
	var position uint32
	for _, r := range sample {
		textRange := directwrite.DWRITE_TEXT_RANGE{StartPosition: position, Length: uint32(utf16.RuneLen(r))}
		position += textRange.Length
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r >= 0xFE00 && r <= 0xFE0F || r >= 0xE0100 && r <= 0xE01EF {
			continue
		}
		codepoints = append(codepoints, uint32(r))
		textRanges = append(textRanges, textRange)
	}
	var missing []directwrite.DWRITE_TEXT_RANGE
	if len(codepoints) > 0 {
		glyphs := make([]uint16, len(codepoints))
		if err := face.GetGlyphIndices(&codepoints[0], uint32(len(codepoints)), &glyphs[0]); err != nil {
			return nil, err
		}
		for i, glyph := range glyphs {
			if glyph == 0 {
				missing = append(missing, textRanges[i])
			}
		}
	}
	var builder *directwrite.IDWriteFontSetBuilder
	if err := factory3.CreateFontSetBuilder(&builder); err != nil {
		return nil, err
	}
	defer builder.Release()
	if err := builder.AddFontFaceReference_(reference); err != nil {
		return nil, err
	}
	var set *directwrite.IDWriteFontSet
	if err := builder.CreateFontSet(&set); err != nil {
		return nil, err
	}
	defer set.Release()
	var collection *directwrite.IDWriteFontCollection1
	if err := factory3.CreateFontCollectionFromFontSet(set, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()
	var family *directwrite.IDWriteFontFamily1
	if err := collection.GetFontFamily(0, &family); err != nil {
		return nil, err
	}
	defer family.Release()
	var names *directwrite.IDWriteLocalizedStrings
	if err := family.GetFamilyNames(&names); err != nil {
		return nil, err
	}
	defer names.Release()
	var nameLength uint32
	if err := names.GetStringLength(0, &nameLength); err != nil {
		return nil, err
	}
	name := make([]uint16, nameLength+1)
	if err := names.GetString(0, foundation.PWSTR(unsafe.Pointer(&name[0])), uint32(len(name))); err != nil {
		return nil, err
	}
	var format *directwrite.IDWriteTextFormat
	if err := factory.CreateTextFormat(string(utf16.Decode(name[:nameLength])), &collection.IDWriteFontCollection, face.GetWeight(), face.GetStyle(), face.GetStretch(), size, "", &format); err != nil {
		return nil, err
	}
	defer format.Release()
	if err := format.SetLineSpacing(directwrite.DWRITE_LINE_SPACING_METHOD_UNIFORM, size*lineHeight, size*1.25); err != nil {
		return nil, err
	}
	layoutWidth := float32(max(1, width-16))
	if width == 0 {
		sample = strings.ReplaceAll(sample, "\n", " ")
		layoutWidth = 1 << 20
		if err := format.SetWordWrapping(directwrite.DWRITE_WORD_WRAPPING_NO_WRAP); err != nil {
			return nil, err
		}
	}
	var layout *directwrite.IDWriteTextLayout
	textLength := uint32(len(utf16.Encode([]rune(sample))))
	layoutHeight := float32(height)
	if height == 0 {
		layoutHeight = 1 << 20
	}
	if err := factory.CreateTextLayout(sample, textLength, format, layoutWidth, layoutHeight, &layout); err != nil {
		return nil, err
	}
	defer layout.Release()
	layout2, err := win32.QueryInterface[directwrite.IDWriteTextLayout2](layout, &directwrite.IID_IDWriteTextLayout2)
	if err != nil {
		return nil, err
	}
	defer layout2.Release()
	// An empty fallback list keeps every glyph tied to the selected file and face.
	var fallbackBuilder *directwrite.IDWriteFontFallbackBuilder
	if err := factory3.CreateFontFallbackBuilder(&fallbackBuilder); err != nil {
		return nil, err
	}
	defer fallbackBuilder.Release()
	var fallback *directwrite.IDWriteFontFallback
	if err := fallbackBuilder.CreateFontFallback(&fallback); err != nil {
		return nil, err
	}
	defer fallback.Release()
	if err := layout2.SetFontFallback(fallback); err != nil {
		return nil, err
	}
	if len(axes) > 0 && textLength > 0 {
		values := make([]directwrite.DWRITE_FONT_AXIS_VALUE, 0, len(axes))
		for tag, value := range axes {
			if len(tag) != 4 {
				return nil, fmt.Errorf("invalid axis tag %q", tag)
			}
			values = append(values, directwrite.DWRITE_FONT_AXIS_VALUE{AxisTag: directwrite.DWRITE_FONT_AXIS_TAG(binary.LittleEndian.Uint32([]byte(tag))), Value: value})
		}
		layout4, err := win32.QueryInterface[directwrite.IDWriteTextLayout4](layout, &directwrite.IID_IDWriteTextLayout4)
		if err != nil {
			return nil, err
		}
		defer layout4.Release()
		if err := layout4.SetFontAxisValues(values, directwrite.DWRITE_TEXT_RANGE{StartPosition: 0, Length: textLength}); err != nil {
			return nil, err
		}
	}
	var originY float32
	if width == 0 || height == 0 {
		var metrics directwrite.DWRITE_TEXT_METRICS
		if err := layout.GetMetrics(&metrics); err != nil {
			return nil, err
		}
		if width == 0 {
			width = int(math.Ceil(float64(metrics.WidthIncludingTrailingWhitespace))) + 16
		}
		if height == 0 {
			if err := layout.SetMaxHeight(max(1, metrics.Height)); err != nil {
				return nil, err
			}
			var overhang directwrite.DWRITE_OVERHANG_METRICS
			if err := layout.GetOverhangMetrics(&overhang); err != nil {
				return nil, err
			}
			inkTop := -overhang.Top
			inkHeight := max(0, metrics.Height+overhang.Bottom-inkTop)
			height = max(1, int(math.Ceil(float64(max(metrics.Height, inkHeight)))))
			originY = (float32(height)-inkHeight)/2 - inkTop
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return drawLayout(ctx, layout, missing, width, height, originY, scale, ink)
}

func drawLayout(ctx context.Context, layout *directwrite.IDWriteTextLayout, missing []directwrite.DWRITE_TEXT_RANGE, width, height int, originY, scale float32, ink color.RGBA) (*image.RGBA, error) {
	pixelWidth := int(math.Ceil(float64(float32(max(1, width)) * scale)))
	pixelHeight := int(math.Ceil(float64(float32(height) * scale)))
	if float64(pixelWidth)*float64(pixelHeight)*4 > maxPreviewBitmapBytes {
		return nil, fmt.Errorf("预览尺寸过大，请减小字号或缩短文案")
	}
	var unknown *win32.IUnknown
	if err := com.CoCreateInstance(&imaging.CLSID_WICImagingFactory, nil, com.CLSCTX_INPROC_SERVER, &imaging.IID_IWICImagingFactory, &unknown); err != nil {
		return nil, err
	}
	wic := (*imaging.IWICImagingFactory)(unsafe.Pointer(unknown))
	defer wic.Release()
	var pixels *imaging.IWICBitmap
	if err := wic.CreateBitmap(uint32(pixelWidth), uint32(pixelHeight), &imaging.GUID_WICPixelFormat32bppPBGRA, imaging.WICBitmapCacheOnLoad, &pixels); err != nil {
		return nil, err
	}
	defer pixels.Release()
	if err := direct2d.D2D1CreateFactory(direct2d.D2D1_FACTORY_TYPE_SINGLE_THREADED, &direct2d.IID_ID2D1Factory, nil, &unknown); err != nil {
		return nil, err
	}
	factory := (*direct2d.ID2D1Factory)(unsafe.Pointer(unknown))
	defer factory.Release()
	properties := direct2d.D2D1_RENDER_TARGET_PROPERTIES{Type: direct2d.D2D1_RENDER_TARGET_TYPE_SOFTWARE, PixelFormat: common.D2D1_PIXEL_FORMAT{Format: dxgicommon.DXGI_FORMAT_B8G8R8A8_UNORM, AlphaMode: common.D2D1_ALPHA_MODE_PREMULTIPLIED}, DpiX: 96 * scale, DpiY: 96 * scale}
	var target *direct2d.ID2D1RenderTarget
	if err := factory.CreateWicBitmapRenderTarget(pixels, &properties, &target); err != nil {
		return nil, err
	}
	defer target.Release()
	var brush *direct2d.ID2D1SolidColorBrush
	if err := target.CreateSolidColorBrush(&common.D2D1_COLOR_F{R: float32(ink.R) / 255, G: float32(ink.G) / 255, B: float32(ink.B) / 255, A: 1}, nil, &brush); err != nil {
		return nil, err
	}
	defer brush.Release()
	if len(missing) > 0 {
		var blank *direct2d.ID2D1SolidColorBrush
		if err := target.CreateSolidColorBrush(&common.D2D1_COLOR_F{}, nil, &blank); err != nil {
			return nil, err
		}
		defer blank.Release()
		// Hide missing glyphs while retaining their layout advances.
		for _, textRange := range missing {
			if err := layout.SetDrawingEffect(&blank.IUnknown, textRange); err != nil {
				return nil, err
			}
		}
	}
	target.SetTextAntialiasMode(direct2d.D2D1_TEXT_ANTIALIAS_MODE_GRAYSCALE)
	target.BeginDraw()
	target.Clear(&common.D2D1_COLOR_F{})
	target.DrawTextLayout(common.D2D_POINT_2F{X: 8, Y: originY}, layout, &brush.ID2D1Brush, direct2d.D2D1_DRAW_TEXT_OPTIONS_NONE)
	if err := target.EndDraw(nil, nil); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bitmap := image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
	if err := pixels.CopyPixels(nil, uint32(bitmap.Stride), bitmap.Pix); err != nil {
		return nil, err
	}
	for i := 0; i < len(bitmap.Pix); i += 4 {
		bitmap.Pix[i], bitmap.Pix[i+2] = bitmap.Pix[i+2], bitmap.Pix[i]
	}
	return bitmap, nil
}
