package app

import (
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

func (a *appState) compareView(c *ui.Context) {
	faces := make([]fonts.Entry, 0, len(a.compareIDs))
	for _, id := range a.compareIDs {
		if index, exists := a.faceIndex[id]; exists {
			faces = append(faces, a.faces[index])
		}
	}
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回浏览").Clicked() {
				a.comparePage = false
			}
			ui.Textf(c, "字体对比 · %d 款", len(faces)).FontSize(22).Bold()
		})
		a.previewToolbar(c)
		a.syncPreviews(c, faces, a.previewText, a.previewSize*1.5, 450, 240)
		ui.Scroll(c).Grow(1).Children(func() {
			for start := 0; start < len(faces); start += 2 {
				end := min(start+2, len(faces))
				ui.Row(c).Gap(16).Children(func() {
					for _, face := range faces[start:end] {
						ui.Column(c).Grow(1).Padding(16).Gap(8).Children(func() {
							ui.Row(c).Gap(8).Children(func() {
								ui.Text(c, face.FullName).FontSize(18).Bold().Grow(1)
								if ui.Button(c, "移除").Clicked() {
									a.compareIDs = slices.DeleteFunc(a.compareIDs, func(id string) bool { return id == face.ID() })
								}
							})
							ui.Textf(c, "%s · 面 %d", face.Path, face.Index+1).SingleLine().TextColor(c.Theme().TextMuted)
							a.previewView(c, face, a.previewSize*1.5, 450, 240)
						}).Border(1, c.Theme().Border).Radius(8)
					}
				})
			}
		})
	})
}

func (a *appState) previewToolbar(c *ui.Context) {
	ui.Row(c).Gap(8).Children(func() {
		if ui.TextInput(c, &a.previewText).Placeholder("输入预览文案").Grow(1).Changed() {
			a.previewPreset = "自定义"
			a.customPreviewText = a.previewText
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
			break
		}
	}
}

func (a *appState) previewKey(face fonts.Entry, sample string, size float32, width, height int, scale float32, ink color.RGBA) string {
	key := fmt.Sprintf("%s:%g:%g:%d:%d:%g:%d:%d:%d:%s", coverageKey(face), size, a.previewLineHeight, width, height, scale, ink.R, ink.G, ink.B, sample)
	axes := a.axisValues[face.ID()]
	tags := make([]string, 0, len(axes))
	for tag := range axes {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	for _, tag := range tags {
		key += fmt.Sprintf(":%s=%.3f", tag, axes[tag])
	}
	return key
}

func (a *appState) ensurePreview(face fonts.Entry, sample string, size float32, width, height int, scale float32, ink color.RGBA) string {
	if a.previewLoading == nil {
		a.previewBitmaps = make(map[string]*ui.Bitmap)
		a.previewLast = make(map[string]*ui.Bitmap)
		a.previewLoading = make(map[string]*previewRequest)
		a.previewErrors = make(map[string]string)
	}
	key := a.previewKey(face, sample, size, width, height, scale, ink)
	if a.previewBitmaps[key] != nil || a.previewLoading[key] != nil || a.previewErrors[key] != "" {
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
		bitmap, err := preview.Render(ctx, face.Path, face.Index, axes, sample, size, lineHeight, width, height, scale, ink)
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
			a.previewBitmaps[key] = ui.NewBitmap(bitmap)
		})
	}()
	return key
}

func (a *appState) cancelPreviews(wanted map[string]bool) {
	for key := range a.previewBitmaps {
		if !wanted[key] {
			delete(a.previewBitmaps, key)
		}
	}
	for slot := range a.previewLast {
		if !a.previewSlots[slot] {
			delete(a.previewLast, slot)
		}
	}
	for key, request := range a.previewLoading {
		if !wanted[key] {
			request.hint.finish()
			request.cancel()
			delete(a.previewLoading, key)
		}
	}
	for key := range a.previewErrors {
		if !wanted[key] {
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

func (a *appState) syncPreview(c *ui.Context, face fonts.Entry, sample string, size float32, width, height int) string {
	textColor := c.Theme().Text
	ink := color.RGBA{R: textColor.R, G: textColor.G, B: textColor.B, A: 255}
	scale := float32(hidpi.GetDpiForWindow(foundation.HWND(a.window.NativeHandle()))) / 96
	key := a.ensurePreview(face, sample, size, width, height, scale, ink)
	a.previewWanted[key] = true
	a.previewSlots[previewSlot(face, width, height, ink)] = true
	return key
}

func (a *appState) previewView(c *ui.Context, face fonts.Entry, size float32, width, height int) {
	a.previewTextView(c, face, a.previewText, size, width, height)
}

func previewSlot(face fonts.Entry, width, height int, ink color.RGBA) string {
	view, id := "list", face.ID()
	if width > 0 {
		view = "compare"
		if height == 0 {
			view = "details"
		}
	}
	return fmt.Sprintf("%s:%s:%d:%d:%d", view, id, ink.R, ink.G, ink.B)
}

func (a *appState) previewTextView(c *ui.Context, face fonts.Entry, sample string, size float32, width, height int) ui.Element {
	textColor := c.Theme().Text
	scale := float32(hidpi.GetDpiForWindow(foundation.HWND(a.window.NativeHandle()))) / 96
	key := a.previewKey(face, sample, size, width, height, scale, color.RGBA{R: textColor.R, G: textColor.G, B: textColor.B, A: 255})
	slot := previewSlot(face, width, height, color.RGBA{R: textColor.R, G: textColor.G, B: textColor.B, A: 255})
	request := a.previewLoading[key]
	loading := request != nil && request.hint.show()
	if bitmap := a.previewBitmaps[key]; bitmap != nil {
		if a.previewSlots[slot] {
			a.previewLast[slot] = bitmap
		}
		displayWidth := float32(width)
		displayHeight := float32(height)
		if width == 0 {
			displayWidth, _ = ui.ImageSize(bitmap)
			displayWidth /= scale
		}
		if height == 0 {
			_, displayHeight = ui.ImageSize(bitmap)
			displayHeight /= scale
		}
		image := ui.Image(c, bitmap).Size(displayWidth, displayHeight).Fit(ui.Contain)
		if width == 0 {
			image.Shrink(0)
		}
		return image
	} else if message := a.previewErrors[key]; message != "" {
		return ui.Text(c, message).TextColor(c.Theme().TextMuted).SingleLine()
	} else if bitmap := a.previewLast[slot]; bitmap != nil {
		displayWidth := float32(width)
		displayHeight := float32(height)
		if width == 0 {
			displayWidth, _ = ui.ImageSize(bitmap)
			displayWidth /= scale
		}
		if height == 0 {
			_, displayHeight = ui.ImageSize(bitmap)
			displayHeight /= scale
		}
		box := ui.Box(c).Size(displayWidth, displayHeight)
		if width == 0 {
			box.Shrink(0)
		}
		return box.Children(func() {
			ui.Image(c, bitmap).Size(displayWidth, displayHeight).Fit(ui.Contain)
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
