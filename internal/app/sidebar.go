package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *appState) treeChildren(node string) []string {
	var children []string
	switch node {
	case "all":
		children = []string{"system", "custom"}
	case "custom":
		children = a.sources
	default:
		children = a.dirs[node]
	}
	if len(children) == 0 {
		return nil
	}
	return children
}

func (a *appState) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(8, 12).Gap(12).Children(func() {
		ui.Text(c, "字体目录").TextColor(c.Theme().TextMuted)
		for row := 0; row < a.tree.Rows(); row++ {
			if a.tree.Item(row) == a.scope {
				a.treeRow = row
				break
			}
		}
		a.tree.List.Selected = &a.treeRow
		a.tree.Prefix = func(row ui.OutlineRow) {
			ui.Box(c).Width(float32(row.Depth) * 16).Shrink(0)
			arrow := row.Arrow().Size(16, 22)
			if row.Branch {
				arrow.Draw(func(p *ui.Painter, r ui.Rect) {
					x, y := r.X+r.W/2, r.Y+r.H/2
					var path ui.Path
					if row.Open {
						path.MoveTo(x-3, y-1.5).LineTo(x, y+1.5).LineTo(x+3, y-1.5)
					} else {
						path.MoveTo(x-1.5, y-3).LineTo(x+1.5, y).LineTo(x-1.5, y+3)
					}
					p.StrokePath(&path, 1.25, c.Theme().Text)
				})
			}
		}
		if ui.Outline(c, &a.tree, []string{"all"}, a.treeChildren, func(node string) {
			label := node
			icon := ""
			isSource := slices.Contains(a.sources, node)
			rowHeight := float32(22)
			switch node {
			case "all":
				label = "全部"
				icon = "\ue8f1"
			case "system":
				label = "系统字体"
				icon = "\ue8d2"
			case "custom":
				label = "自定义文件夹"
			default:
				label = filepath.Base(node)
				if label == string(filepath.Separator) {
					label = node
				}
				if isSource {
					rowHeight = 42
				}
			}
			row := ui.Row(c).Grow(1).MinWidth(0).Height(rowHeight).Padding(0, 6, 0, 0).Gap(4).AlignItems(ui.Center).Children(func() {
				if icon != "" {
					ui.Text(c, icon).Font("Segoe MDL2 Assets").FontSize(13).Width(16).Shrink(0)
				} else {
					ui.Box(c).Size(16, 16).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
						ink := c.Theme().TextMuted
						if node == a.scope {
							ink = c.Theme().AccentText
						}
						x, y := r.X+1, r.Y+3
						var folder ui.Path
						folder.MoveTo(x, y).LineTo(x+5, y).LineTo(x+7, y+2).LineTo(x+13, y+2).LineTo(x+13, y+10).LineTo(x, y+10).Close()
						p.FillPath(&folder, ink)
						if a.tree.Open.Has(node) && len(a.treeChildren(node)) > 0 {
							var flap ui.Path
							flap.MoveTo(x+1, y+4).LineTo(x+12, y+4)
							p.StrokePath(&flap, 1, c.Theme().Surface)
						}
					})
				}
				if isSource {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, label).FontSize(12).SingleLine().Tooltip(node)
						ink := c.Theme().TextMuted
						if node == a.scope {
							ink = c.Theme().AccentText
						}
						ui.Text(c, node).FontSize(12).TextColor(ink).SingleLine().Tooltip(node)
					})
				} else {
					ui.Text(c, label).FontSize(12).SingleLine().Grow(1).MinWidth(0).Tooltip(label)
				}
				ui.Textf(c, "%d", a.count(node)).FontSize(12).SingleLine().Shrink(0)
			})
			if node == "custom" {
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item("添加文件夹…").Chosen() {
						a.chooseFolder()
					}
				})
			} else if isSource {
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item("移除该文件夹").Chosen() {
						a.removeFolder(node)
					}
				})
			}
		}).Grow(1).Changed() {
			a.scope = a.tree.Item(a.treeRow)
			a.rebuild()
			a.table.ScrollTo(0, ui.Start)
			a.saveSettings()
		}
		if ui.Button(c, "刷新").Clicked() {
			a.refresh()
		}
		if a.scanning {
			ui.Text(c, "正在扫描…").TextColor(c.Theme().TextMuted)
			if ui.Button(c, "取消扫描").Clicked() {
				a.cancelScan()
			}
		}
		if len(a.errors) > 0 && ui.Button(c, fmt.Sprintf("读取错误 (%d)", len(a.errors))).Clicked() {
			a.showErrors = !a.showErrors
		}
		if a.showErrors {
			ui.Scroll(c).Height(160).Children(func() {
				for _, message := range a.errors {
					ui.Text(c, message).FontSize(12)
				}
			})
		}
		themes := []string{"跟随系统", "浅色", "深色"}
		if ui.Button(c, "主题："+themes[a.themeMode]).Clicked() {
			a.themeMode = (a.themeMode + 1) % len(themes)
			a.saveSettings()
		}
	})
}

func (a *appState) removeFolder(path string) {
	a.sources = slices.DeleteFunc(a.sources, func(source string) bool { return source == path })
	a.scope = "custom"
	a.saveSettings()
	a.refresh()
}

func (a *appState) chooseFolder() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "添加字体文件夹", Directory: true})
		a.window.Update(func() {
			if err != nil {
				a.errors = append(a.errors, err.Error())
				return
			}
			for _, path := range paths {
				a.addFolder(path)
			}
		})
	}()
}

func (a *appState) addFolder(input string) {
	path := strings.TrimSpace(input)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		a.errors = append(a.errors, "文件夹不可访问: "+path)
		return
	}
	path, _ = filepath.Abs(path)
	if slices.Contains(a.sources, path) {
		return
	}
	a.sources = append(a.sources, path)
	a.saveSettings()
	a.refresh()
}
