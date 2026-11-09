package preview

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"fontlens/internal/fonts"
	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/directwrite"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

type oneFace struct{ face *font.Face }

func (f oneFace) ResolveFace(r rune) *font.Face { return f.face }

var renderSlots = make(chan struct{}, 1)

func Render(ctx context.Context, path string, faceIndex int, axes map[string]float32, sample string, size, lineHeight float32, width, height int, ink color.RGBA) (*image.RGBA, error) {
	if width <= 0 || height <= 0 || size <= 0 {
		return nil, fmt.Errorf("invalid preview dimensions")
	}
	// Each render parses font outlines; bound concurrent copies of large fonts.
	select {
	case renderSlots <- struct{}{}:
		defer func() { <-renderSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
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
	factory2, err := win32.QueryInterface[directwrite.IDWriteFactory2](factory, &directwrite.IID_IDWriteFactory2)
	if err != nil {
		return nil, err
	}
	defer factory2.Release()
	var dwFile *directwrite.IDWriteFontFile
	var resource opentype.Resource
	var sfnt []byte
	var file *os.File
	if strings.EqualFold(filepath.Ext(path), ".woff2") || strings.EqualFold(filepath.Ext(path), ".woff") {
		sfnt, err = fonts.DecodeWebFont(path)
		if err != nil {
			return nil, err
		}
		resource = bytes.NewReader(sfnt)
		factory5, queryErr := win32.QueryInterface[directwrite.IDWriteFactory5](factory, &directwrite.IID_IDWriteFactory5)
		if queryErr != nil {
			return nil, queryErr
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
		if err := loader.CreateInMemoryFontFileReference(factory, sfnt, nil, &dwFile); err != nil {
			return nil, err
		}
	} else {
		if err := factory.CreateFontFileReference(path, nil, &dwFile); err != nil {
			return nil, err
		}
		file, err = os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		resource = file
	}
	defer dwFile.Release()
	var supported foundation.BOOL
	var fileType directwrite.DWRITE_FONT_FILE_TYPE
	var faceType directwrite.DWRITE_FONT_FACE_TYPE
	var faceCount uint32
	if err := dwFile.Analyze(&supported, &fileType, &faceType, &faceCount); err != nil {
		return nil, err
	}
	if supported == 0 || faceIndex < 0 || uint32(faceIndex) >= faceCount {
		return nil, fmt.Errorf("DirectWrite cannot open face %d", faceIndex)
	}
	var dwFace *directwrite.IDWriteFontFace
	if len(axes) == 0 {
		if err := factory.CreateFontFace(faceType, []*directwrite.IDWriteFontFile{dwFile}, uint32(faceIndex), directwrite.DWRITE_FONT_SIMULATIONS_NONE, &dwFace); err != nil {
			return nil, err
		}
	} else {
		factory6, queryErr := win32.QueryInterface[directwrite.IDWriteFactory6](factory, &directwrite.IID_IDWriteFactory6)
		if queryErr != nil {
			return nil, queryErr
		}
		defer factory6.Release()
		values := make([]directwrite.DWRITE_FONT_AXIS_VALUE, 0, len(axes))
		for tag, value := range axes {
			if len(tag) != 4 {
				return nil, fmt.Errorf("invalid axis tag %q", tag)
			}
			values = append(values, directwrite.DWRITE_FONT_AXIS_VALUE{AxisTag: directwrite.DWRITE_FONT_AXIS_TAG(binary.LittleEndian.Uint32([]byte(tag))), Value: value})
		}
		var reference *directwrite.IDWriteFontFaceReference1
		if err := factory6.CreateFontFaceReference(dwFile, uint32(faceIndex), directwrite.DWRITE_FONT_SIMULATIONS_NONE, values, &reference); err != nil {
			return nil, err
		}
		defer reference.Release()
		var face5 *directwrite.IDWriteFontFace5
		if err := reference.CreateFontFace(&face5); err != nil {
			return nil, err
		}
		dwFace = (*directwrite.IDWriteFontFace)(unsafe.Pointer(face5))
	}
	defer dwFace.Release()
	loaders, err := opentype.NewLoaders(resource)
	if err != nil {
		return nil, err
	}
	if faceIndex >= len(loaders) {
		return nil, fmt.Errorf("face index %d is out of range", faceIndex)
	}
	parsed, err := font.NewFont(loaders[faceIndex])
	if err != nil {
		return nil, err
	}
	goFace := font.NewFace(parsed)
	if len(axes) > 0 {
		variations := make([]font.Variation, 0, len(axes))
		for tag, value := range axes {
			variations = append(variations, font.Variation{Tag: opentype.MustNewTag(tag), Value: value})
		}
		goFace.SetVariations(variations)
	}
	var segmenter shaping.Segmenter
	var shaper shaping.HarfbuzzShaper
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	lineIndex := 0
	for _, paragraph := range strings.Split(sample, "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			lineIndex++
			continue
		}
		input := shaping.Input{Text: runes, RunStart: 0, RunEnd: len(runes), Face: goFace, Size: fixed.Int26_6(size * 64)}
		runs := segmenter.Split(input, oneFace{input.Face})
		outputs := make([]shaping.Output, 0, len(runs))
		for _, run := range runs {
			outputs = append(outputs, shaper.Shape(run))
		}
		var wrapper shaping.LineWrapper
		lines, _ := wrapper.WrapParagraphF(shaping.WrapConfig{Direction: runs[0].Direction}, fixed.Int26_6((width-16)*64), runes, shaping.NewSliceIterator(outputs))
		for _, line := range lines {
			penX := float32(8)
			baseline := size*1.25 + float32(lineIndex)*size*lineHeight
			for _, out := range line {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if len(out.Glyphs) == 0 {
					continue
				}
				ids := make([]uint16, len(out.Glyphs))
				advances := make([]float32, len(out.Glyphs))
				offsets := make([]directwrite.DWRITE_GLYPH_OFFSET, len(out.Glyphs))
				for i, glyph := range out.Glyphs {
					if glyph.GlyphID == 0 {
						return nil, fmt.Errorf("缺字：U+%04X", runes[glyph.ClusterIndex])
					}
					ids[i] = uint16(glyph.GlyphID)
					advances[i] = float32(glyph.Advance) / 64
					offsets[i] = directwrite.DWRITE_GLYPH_OFFSET{AdvanceOffset: float32(glyph.XOffset) / 64, AscenderOffset: float32(glyph.YOffset) / 64}
				}
				glyphRun := directwrite.DWRITE_GLYPH_RUN{FontFace: dwFace, FontEmSize: size, GlyphCount: uint32(len(ids)), GlyphIndices: &ids[0], GlyphAdvances: &advances[0], GlyphOffsets: &offsets[0]}
				var analysis *directwrite.IDWriteGlyphRunAnalysis
				if err := factory2.CreateGlyphRunAnalysis(&glyphRun, nil, directwrite.DWRITE_RENDERING_MODE_NATURAL, directwrite.DWRITE_MEASURING_MODE_NATURAL, directwrite.DWRITE_GRID_FIT_MODE_DEFAULT, directwrite.DWRITE_TEXT_ANTIALIAS_MODE_GRAYSCALE, penX, baseline, &analysis); err != nil {
					return nil, err
				}
				var bounds foundation.RECT
				err = analysis.GetAlphaTextureBounds(directwrite.DWRITE_TEXTURE_ALIASED_1x1, &bounds)
				if err == nil && bounds.Right > bounds.Left && bounds.Bottom > bounds.Top {
					alpha := make([]byte, int(bounds.Right-bounds.Left)*int(bounds.Bottom-bounds.Top))
					err = analysis.CreateAlphaTexture(directwrite.DWRITE_TEXTURE_ALIASED_1x1, &bounds, alpha)
					if err == nil {
						paint(result, bounds, alpha, ink)
					}
				}
				analysis.Release()
				if err != nil {
					return nil, err
				}
				for _, advance := range advances {
					penX += advance
				}
				runtime.KeepAlive(ids)
				runtime.KeepAlive(advances)
				runtime.KeepAlive(offsets)
			}
			lineIndex++
		}
	}
	return result, nil
}

func paint(dst *image.RGBA, bounds foundation.RECT, alpha []byte, ink color.RGBA) {
	width := int(bounds.Right - bounds.Left)
	for y := int(bounds.Top); y < int(bounds.Bottom); y++ {
		for x := int(bounds.Left); x < int(bounds.Right); x++ {
			if !image.Pt(x, y).In(dst.Rect) {
				continue
			}
			a := alpha[(y-int(bounds.Top))*width+x-int(bounds.Left)]
			dst.SetRGBA(x, y, color.RGBA{R: uint8(uint16(ink.R) * uint16(a) / 255), G: uint8(uint16(ink.G) * uint16(a) / 255), B: uint8(uint16(ink.B) * uint16(a) / 255), A: a})
		}
	}
}
