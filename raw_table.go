package main

import (
	"fmt"
	"strings"

	"fontlens/internal/fonts"
	"github.com/egoist/mygo/ui"
)

func (a *appState) openRawTable(face fonts.Entry, tag string) {
	a.rawFace, a.rawTag, a.rawData, a.rawError, a.rawPage = face, tag, nil, "", 0
	a.rawViewOpen = true
	go func() {
		data, err := fonts.ReadRawTable(face.Path, face.Index, tag)
		a.window.Update(func() {
			if !a.rawViewOpen || a.rawFace.ID() != face.ID() || a.rawTag != tag {
				return
			}
			if err != nil {
				a.rawError = err.Error()
			} else {
				a.rawData = data
			}
		})
	}()
}

func (a *appState) rawTableView(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回详情").Clicked() {
				a.rawViewOpen = false
				a.rawData = nil
			}
			ui.Textf(c, "%s · %s 表原始数据", a.rawFace.FullName, a.rawTag).FontSize(20).Bold()
		})
		ui.Textf(c, "%s · 面 %d", a.rawFace.Path, a.rawFace.Index+1).SingleLine()
		if a.rawError != "" {
			ui.Text(c, a.rawError)
			return
		}
		if a.rawData == nil {
			ui.Text(c, "正在读取表数据…")
			return
		}
		const pageSize = 256
		pages := max(1, (len(a.rawData)+pageSize-1)/pageSize)
		ui.Row(c).Gap(12).Children(func() {
			ui.Textf(c, "%d bytes · 第 %d / %d 页", len(a.rawData), a.rawPage+1, pages)
			if ui.Button(c, "上一页").Clicked() && a.rawPage > 0 {
				a.rawPage--
			}
			if ui.Button(c, "下一页").Clicked() && a.rawPage+1 < pages {
				a.rawPage++
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			start := a.rawPage * pageSize
			end := min(start+pageSize, len(a.rawData))
			for offset := start; offset < end; offset += 16 {
				last := min(offset+16, end)
				var hex, ascii strings.Builder
				for _, b := range a.rawData[offset:last] {
					fmt.Fprintf(&hex, "%02X ", b)
					if b >= 32 && b < 127 {
						ascii.WriteByte(b)
					} else {
						ascii.WriteByte('.')
					}
				}
				ui.Textf(c, "%08X  %-48s  %s", offset, hex.String(), ascii.String()).Font("monospace")
			}
		})
	})
}
