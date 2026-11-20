package app

import (
	"fmt"
	"strings"

	"fontlens/internal/fonts"
	"github.com/egoist/mygo/ui"
)

func (a *appState) closeRawTable() {
	a.rawLoading.finish()
	a.rawViewOpen = false
	a.rawPages, a.rawData = nil, nil
}

func (a *appState) openRawTable(face fonts.Entry, table fonts.Table) {
	a.rawFace, a.rawTable, a.rawPages = face, table, nil
	a.rawViewOpen = true
	a.loadRawPage(0)
}

func (a *appState) loadRawPage(page int) {
	a.rawLoading.finish()
	hint := newLoadingHint(a.window)
	a.rawLoading = hint
	a.rawPage, a.rawData, a.rawError = page, nil, ""
	face, table, pages := a.rawFace, a.rawTable, a.rawPages
	go func() {
		var err error
		if pages == nil {
			pages, err = fonts.OpenTablePages(face, table)
		}
		var data []byte
		if err == nil {
			data, err = pages.ReadPage(page)
		}
		a.window.Update(func() {
			hint.finish()
			if !a.rawViewOpen || a.rawLoading != hint {
				return
			}
			if err != nil {
				a.rawError = err.Error()
			} else {
				a.rawPages, a.rawData = pages, data
			}
		})
	}()
}

func (a *appState) rawTableView(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回详情").Clicked() {
				a.closeRawTable()
			}
			ui.Textf(c, "%s · %s 表原始数据", a.rawFace.FullName, a.rawTable.Tag).FontSize(20).Bold()
		})
		ui.Textf(c, "%s · 面 %d", a.rawFace.Path, a.rawFace.Index+1).SingleLine()
		if a.rawError != "" {
			ui.Text(c, a.rawError)
			return
		}
		pages := max(1, (a.rawTable.Size+fonts.TablePageSize-1)/fonts.TablePageSize)
		ui.Row(c).Gap(12).Children(func() {
			ui.Textf(c, "%d bytes · 第 %d / %d 页", a.rawTable.Size, a.rawPage+1, pages)
			if ui.Button(c, "上一页").Disabled(a.rawPages == nil || a.rawPage == 0).Clicked() {
				a.loadRawPage(a.rawPage - 1)
			}
			if ui.Button(c, "下一页").Disabled(a.rawPages == nil || a.rawPage+1 >= pages).Clicked() {
				a.loadRawPage(a.rawPage + 1)
			}
		})
		if a.rawData == nil {
			if a.rawLoading.show() {
				ui.Text(c, "正在读取表数据…")
			}
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			for offset := 0; offset < len(a.rawData); offset += 16 {
				last := min(offset+16, len(a.rawData))
				var hex, ascii strings.Builder
				for _, b := range a.rawData[offset:last] {
					fmt.Fprintf(&hex, "%02X ", b)
					if b >= 32 && b < 127 {
						ascii.WriteByte(b)
					} else {
						ascii.WriteByte('.')
					}
				}
				ui.Textf(c, "%08X  %-48s  %s", a.rawPage*fonts.TablePageSize+offset, hex.String(), ascii.String()).Font("monospace")
			}
		})
	})
}
