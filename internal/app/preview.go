package app

import (
	"container/list"
	"context"
	"fmt"
	"image/color"
	"math"
	"slices"

	"fontlens/internal/fonts"
	"fontlens/internal/preview"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/hidpi"
	"github.com/egoist/mygo/ui"
)

type previewRequest struct {
	cancel context.CancelFunc
	hint   *loadingHint
}

const (
	familyPreviewBudget int64 = 4 << 20
	samplePreviewBudget int64 = 28 << 20
)

type previewSlot struct {
	font, view     string
	size, modified int64
}

type previewKey struct {
	slot     previewSlot
	settings string
}

type displayedPreview struct {
	bitmap        *ui.Bitmap
	width, height float32
	offsetY       float32
}

type bitmapCache struct {
	recent map[previewKey]*list.Element
	order  list.List
	bytes  int64
}

type cachedBitmap struct {
	key    previewKey
	bitmap *ui.Bitmap
	bytes  int64
}

func (cache *bitmapCache) put(key previewKey, bitmap *ui.Bitmap) {
	if cache.recent == nil {
		cache.recent = make(map[previewKey]*list.Element)
	}
	if previous := cache.recent[key]; previous != nil {
		cache.remove(previous)
	}
	width, height := bitmap.Size()
	entry := cachedBitmap{key: key, bitmap: bitmap, bytes: int64(width) * int64(height) * 4}
	cache.recent[key] = cache.order.PushFront(entry)
	cache.bytes += entry.bytes
}

func (cache *bitmapCache) bitmap(key previewKey) *ui.Bitmap {
	if item := cache.recent[key]; item != nil {
		return item.Value.(cachedBitmap).bitmap
	}
	return nil
}

func (cache *bitmapCache) touch(key previewKey) {
	if item := cache.recent[key]; item != nil {
		cache.order.MoveToFront(item)
	}
}

func (cache *bitmapCache) remove(item *list.Element) {
	entry := cache.order.Remove(item).(cachedBitmap)
	delete(cache.recent, entry.key)
	cache.bytes -= entry.bytes
}

func (cache *bitmapCache) trim(wanted map[previewKey]bool, pinned map[*ui.Bitmap]bool, budget int64) {
	// The budget counts base RGBA pixels; displayed and fallback previews stay pinned.
	for item := cache.order.Back(); item != nil && cache.bytes > budget; {
		previous := item.Prev()
		entry := item.Value.(cachedBitmap)
		if !wanted[entry.key] && !pinned[entry.bitmap] {
			cache.remove(item)
		}
		item = previous
	}
}

func (a *appState) previewCache(key previewKey) *bitmapCache {
	if key.slot.view == "list" {
		return &a.familyPreviews
	}
	return &a.samplePreviews
}

func (a *appState) compareView(c *ui.Context) {
	const outerPadding, gap, cardPadding, border = 20, 16, 16, 1
	faces := make([]fonts.Entry, 0, len(a.compareIDs))
	for _, id := range a.compareIDs {
		if index, exists := a.faceIndex[id]; exists {
			faces = append(faces, a.faces[index])
		}
	}
	width, _ := c.Size()
	columns := max(1, min(2, len(faces)))
	cardWidth := (width - 2*outerPadding - float32(columns-1)*gap) / float32(columns)
	previewWidth := max(1, int(cardWidth-2*(cardPadding+border)))
	ui.Column(c).Fill().Padding(outerPadding).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回浏览").Clicked() {
				a.comparePage = false
			}
			ui.Textf(c, "字体对比 · %d 款", len(faces)).FontSize(22).Bold()
		})
		a.previewToolbar(c)
		a.syncPreviews(c, faces, a.previewText, a.previewSize, previewWidth, 0)
		ui.Scroll(c).Grow(1).Gap(gap).Children(func() {
			for start := 0; start < len(faces); start += columns {
				end := min(start+columns, len(faces))
				ui.Row(c).Gap(gap).AlignItems(ui.Stretch).Children(func() {
					for _, face := range faces[start:end] {
						ui.Column(c).Width(cardWidth).Padding(cardPadding).Gap(8).Children(func() {
							ui.Row(c).Gap(8).Children(func() {
								ui.Text(c, face.FullName).FontSize(18).Bold().Grow(1)
								if ui.Button(c, "移除").Clicked() {
									a.compareIDs = slices.DeleteFunc(a.compareIDs, func(id string) bool { return id == face.ID() })
								}
							})
							ui.Textf(c, "%s · 面 %d", face.Path, face.Index+1).SingleLine().TextColor(c.Theme().TextMuted)
							a.previewView(c, face, a.previewSize, previewWidth, 0)
						}).Border(border, c.Theme().Border).Radius(8)
					}
				})
			}
		})
	})
}

func (a *appState) previewToolbar(c *ui.Context) {
	ui.Row(c).Gap(8).Children(func() {
		if ui.TextInput(c, &a.previewText).Placeholder("输入预览文案").Grow(1).Changed() {
			if a.previewPreset == "自定义" {
				a.customPreviewText = a.previewText
			} else {
				if a.previewOverrides == nil {
					a.previewOverrides = make(map[string]string)
				}
				a.previewOverrides[a.previewPreset] = a.previewText
			}
			a.saveSettings()
		}
		labels := []string{"自定义"}
		for _, preset := range preview.Presets {
			labels = append(labels, preset.Label)
		}
		if ui.Select(c, &a.previewPreset, labels).Changed() {
			a.selectPreviewPreset(a.previewPreset)
			a.saveSettings()
		}
		_, edited := a.previewOverrides[a.previewPreset]
		if ui.Button(c, "恢复默认文案").Disabled(!edited).Clicked() {
			delete(a.previewOverrides, a.previewPreset)
			a.selectPreviewPreset(a.previewPreset)
			a.saveSettings()
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "字号 −").Clicked() && a.previewSize > 14 {
			a.previewSize -= 2
		}
		ui.Textf(c, "%.0f px", a.previewSize)
		if ui.Button(c, "字号 +").Clicked() && a.previewSize < 48 {
			a.previewSize += 2
		}
		if ui.Button(c, "行距 −").Clicked() {
			a.previewLineHeight = max(1.0, a.previewLineHeight-0.1)
		}
		ui.Textf(c, "行距 %.1f×", a.previewLineHeight)
		if ui.Button(c, "行距 +").Clicked() {
			a.previewLineHeight = min(2.0, a.previewLineHeight+0.1)
		}
	})
}

func (a *appState) selectPreviewPreset(label string) {
	a.previewPreset = "自定义"
	a.previewText = a.customPreviewText
	for _, preset := range preview.Presets {
		if preset.Label == label {
			a.previewPreset = label
			a.previewText = preset.Text
			if text, edited := a.previewOverrides[label]; edited {
				a.previewText = text
			}
			break
		}
	}
}

func (a *appState) previewKey(face fonts.Entry, sample string, size float32, width, height int, scale float32) previewKey {
	settings := fmt.Sprintf("%g:%g:%d:%d:%g:%s", size, a.previewLineHeight, width, height, scale, sample)
	axes := a.axisValues[face.ID()]
	tags := make([]string, 0, len(axes))
	for tag := range axes {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	for _, tag := range tags {
		settings += fmt.Sprintf(":%s=%.3f", tag, axes[tag])
	}
	view := "list"
	if width > 0 {
		view = "details"
		if a.comparePage {
			view = "compare"
		}
	}
	return previewKey{slot: previewSlot{font: face.ID(), size: face.Size, modified: face.Modified, view: view}, settings: settings}
}

func (a *appState) ensurePreview(face fonts.Entry, sample string, size float32, width, height int, scale float32) previewKey {
	if a.previewLoading == nil {
		a.previewLast = make(map[previewSlot]displayedPreview)
		a.previewLoading = make(map[previewKey]*previewRequest)
		a.previewErrors = make(map[previewKey]string)
	}
	key := a.previewKey(face, sample, size, width, height, scale)
	cache := a.previewCache(key)
	if cache.bitmap(key) != nil || a.previewLoading[key] != nil || a.previewErrors[key] != "" {
		cache.touch(key)
		return key
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := &previewRequest{cancel: cancel, hint: newLoadingHint(a.window)}
	a.previewLoading[key] = request
	lineHeight := a.previewLineHeight
	axes := make(map[string]float32)
	for tag, value := range a.axisValues[face.ID()] {
		axes[tag] = float32(value)
	}
	go func() {
		// Cache white glyphs and tint them when drawing so theme animation reuses the bitmap.
		bitmap, err := preview.Render(ctx, face.Path, face.Index, axes, sample, size, lineHeight, width, height, scale, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		if ctx.Err() != nil {
			cancel()
			return
		}
		a.window.Update(func() {
			defer cancel()
			request.hint.finish()
			if a.previewLoading[key] != request {
				return
			}
			delete(a.previewLoading, key)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				a.previewErrors[key] = err.Error()
				return
			}
			cache.put(key, ui.NewBitmap(bitmap))
		})
	}()
	return key
}

func (a *appState) cancelPreview(key previewKey, request *previewRequest) {
	request.hint.finish()
	request.cancel()
	delete(a.previewLoading, key)
}

func (a *appState) cancelPreviews(wanted map[previewKey]bool) {
	for slot := range a.previewLast {
		if !a.previewSlots[slot] {
			delete(a.previewLast, slot)
		}
	}
	for key, request := range a.previewLoading {
		if !wanted[key] {
			a.cancelPreview(key, request)
		}
	}
	for key := range a.previewErrors {
		if !wanted[key] {
			delete(a.previewErrors, key)
		}
	}
	if a.familyPreviews.bytes > familyPreviewBudget || a.samplePreviews.bytes > samplePreviewBudget {
		pinned := make(map[*ui.Bitmap]bool, len(a.previewLast))
		for _, last := range a.previewLast {
			pinned[last.bitmap] = true
		}
		a.familyPreviews.trim(wanted, pinned, familyPreviewBudget)
		a.samplePreviews.trim(wanted, pinned, samplePreviewBudget)
	}
}

func (a *appState) pruneFamilyPreviews() {
	retained := func(slot previewSlot) bool {
		index, ok := a.faceIndex[slot.font]
		if !ok {
			return false
		}
		entry := &a.browserEntries[index]
		return entry.inResults && entry.face.Family != "" && entry.face.Size == slot.size && entry.face.Modified == slot.modified
	}
	for key, item := range a.familyPreviews.recent {
		if !retained(key.slot) {
			a.familyPreviews.remove(item)
		}
	}
	for slot := range a.previewLast {
		if slot.view == "list" && !retained(slot) {
			delete(a.previewLast, slot)
		}
	}
	for key, request := range a.previewLoading {
		if key.slot.view == "list" && !retained(key.slot) {
			a.cancelPreview(key, request)
		}
	}
	for key := range a.previewErrors {
		if key.slot.view == "list" && !retained(key.slot) {
			delete(a.previewErrors, key)
		}
	}
}

func (a *appState) syncPreviews(c *ui.Context, faces []fonts.Entry, sample string, size float32, width, height int) {
	for _, face := range faces {
		text := sample
		if width == 0 {
			text = face.Family
			if text == "" {
				continue
			}
		}
		a.syncPreview(c, face, text, size, width, height)
	}
}

func (a *appState) syncPreview(c *ui.Context, face fonts.Entry, sample string, size float32, width, height int) previewKey {
	scale := float32(hidpi.GetDpiForWindow(foundation.HWND(a.window.NativeHandle()))) / 96
	key := a.ensurePreview(face, sample, size, width, height, scale)
	a.previewWanted[key] = true
	a.previewSlots[key.slot] = true
	return key
}

func (a *appState) previewView(c *ui.Context, face fonts.Entry, size float32, width, height int) {
	a.previewTextView(c, face, a.previewText, size, width, height)
}

func (a *appState) previewTextView(c *ui.Context, face fonts.Entry, sample string, size float32, width, height int) ui.Element {
	scale := float32(hidpi.GetDpiForWindow(foundation.HWND(a.window.NativeHandle()))) / 96
	key := a.previewKey(face, sample, size, width, height, scale)
	slot := key.slot
	cache := a.previewCache(key)
	a.previewWanted[key] = true
	a.previewSlots[slot] = true
	request := a.previewLoading[key]
	loading := request != nil && request.hint.show()
	if bitmap := cache.bitmap(key); bitmap != nil {
		cache.touch(key)
		frame := displayedPreview{bitmap: bitmap, width: float32(width), height: float32(height)}
		if width == 0 {
			pixelWidth, _ := ui.ImageSize(bitmap)
			frame.width = pixelWidth / scale
			frame.offsetY = -size * 0.2
		}
		if height == 0 {
			_, pixelHeight := ui.ImageSize(bitmap)
			frame.height = pixelHeight / scale
		}
		a.previewLast[slot] = frame
		image := frame.view(c)
		if width == 0 {
			image.Shrink(0)
		}
		return image
	} else if message := a.previewErrors[key]; message != "" {
		return ui.Text(c, message).TextColor(c.Theme().TextMuted).SingleLine()
	} else if last := a.previewLast[slot]; last.bitmap != nil {
		box := ui.Box(c).Size(last.width, last.height)
		if width == 0 {
			box.Shrink(0).Height(float32(height))
		} else if height == 0 {
			box.Width(float32(width)).Clip()
		}
		return box.Children(func() {
			image := last.view(c)
			if width == 0 || height == 0 {
				image.Absolute().Left(0).Top(0)
			}
			if loading {
				ui.Text(c, "更新中…").Absolute().Right(4).Bottom(2).FontSize(12).TextColor(c.Theme().TextMuted)
			}
		})
	} else {
		displayHeight := float32(height)
		if height == 0 {
			displayHeight = float32(math.Ceil(float64(size * a.previewLineHeight)))
		}
		box := ui.Box(c).Height(displayHeight)
		if width > 0 {
			box.Width(float32(width))
		}
		return box.Children(func() {
			if loading {
				ui.Text(c, "预览生成中…").TextColor(c.Theme().TextMuted)
			}
		})
	}
}

func (frame displayedPreview) view(c *ui.Context) ui.Element {
	return ui.Box(c).Size(frame.width, frame.height).Draw(func(p *ui.Painter, r ui.Rect) {
		w, h := ui.ImageSize(frame.bitmap)
		scale := min(r.W/w, r.H/h)
		// Keep the list's baseline adjustment in the pixel-snapped drawing path.
		dst := ui.Rect{X: r.X + (r.W-w*scale)/2, Y: r.Y + (r.H-h*scale)/2 + frame.offsetY, W: w * scale, H: h * scale}
		p.ImageRegion(frame.bitmap, ui.Rect{}, dst, ui.ImageOptions{Tint: p.Theme().Text})
	})
}
