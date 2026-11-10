package app

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"fontlens/internal/fonts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func (a *appState) inScope(face fonts.Entry) bool {
	switch a.scope {
	case "all":
		return true
	case "system":
		return a.systemIDs[face.ID()]
	case "custom":
		for _, root := range a.sources {
			if within(root, face.Path) {
				return true
			}
		}
		return false
	default:
		return within(a.scope, face.Path)
	}
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *appState) rebuild() {
	a.rows = a.rows[:0]
	seen := make(map[string]struct{})
	for _, face := range a.faces {
		if a.inScope(face) && (a.format == "" || face.Format == a.format) && strings.Contains(strings.ToLower(face.FullName), strings.ToLower(a.query)) {
			key := fontNameKey(face)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			a.rows = append(a.rows, face)
		}
	}
	if a.sortMode != 0 {
		nameOrder := collate.New(language.Chinese, collate.IgnoreCase, collate.Numeric)
		slices.SortStableFunc(a.rows, func(left, right fonts.Entry) int {
			order := nameOrder.CompareString(left.FullName, right.FullName)
			if order == 0 {
				order = strings.Compare(strings.ToLower(left.Path), strings.ToLower(right.Path))
			}
			if order == 0 {
				order = left.Index - right.Index
			}
			if a.sortMode == 2 && !strings.EqualFold(left.FullName, right.FullName) {
				return -order
			}
			return order
		})
	}
	a.selectedRow = -1
	for i := range a.rows {
		if a.rows[i].ID() == a.selectedID {
			a.selectedRow = i
			break
		}
	}
	if a.selectedRow < 0 {
		a.selectedID = ""
		a.releaseDetails()
	}
}

func (a *appState) recount() {
	a.scopeCounts = make(map[string]map[string]struct{})
	for _, face := range a.faces {
		a.addCount(face)
	}
}

func (a *appState) addCount(face fonts.Entry) {
	key := fontNameKey(face)
	a.addScopeCount("all", key)
	if a.systemIDs[face.ID()] {
		a.addScopeCount("system", key)
	}
	for _, root := range a.sources {
		if within(root, face.Path) {
			a.addScopeCount("custom", key)
			break
		}
	}
	for directory := filepath.Dir(face.Path); ; directory = filepath.Dir(directory) {
		a.addScopeCount(directory, key)
		if parent := filepath.Dir(directory); parent == directory {
			break
		}
	}
}

func fontNameKey(face fonts.Entry) string {
	if name := strings.TrimSpace(face.FullName); name != "" {
		return strings.ToLower(name)
	}
	return face.ID()
}

func (a *appState) addScopeCount(scope, key string) {
	if a.scopeCounts[scope] == nil {
		a.scopeCounts[scope] = make(map[string]struct{})
	}
	a.scopeCounts[scope][key] = struct{}{}
}

func (a *appState) count(scope string) int {
	return len(a.scopeCounts[scope])
}

func (a *appState) browser(c *ui.Context, width float32) {
	if a.tableSort != a.sortSeen {
		a.sortMode = (a.sortMode + 1) % 3
		switch a.sortMode {
		case 1:
			a.tableSort = ui.SortOrder{Column: "name"}
		case 2:
			a.tableSort = ui.SortOrder{Column: "name", Descending: true}
		default:
			a.tableSort = ui.SortOrder{}
		}
		a.sortSeen = a.tableSort
		a.rebuild()
		a.table.ScrollTo(0, ui.Start)
	}
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.SearchField(c, &a.query).Placeholder("搜索字体全名").Grow(1).Changed() {
				a.rebuild()
			}
			if ui.Button(c, fmt.Sprintf("对比 (%d)", len(a.compareIDs))).Clicked() && len(a.compareIDs) >= 2 {
				a.comparePage = true
			}
			formats := []string{"", "TTF", "OTF", "TTC", "OTC", "WOFF", "WOFF2"}
			filterLabel := "全部格式"
			if a.format != "" {
				filterLabel = a.format
			}
			if ui.Button(c, filterLabel).Clicked() {
				index := slices.Index(formats, a.format)
				a.format = formats[(index+1)%len(formats)]
				a.rebuild()
			}
		})
		if a.compareLimit {
			ui.Text(c, "最多选择 4 款字体进行对比").TextColor(c.Theme().TextMuted)
		}
		a.previewToolbar(c)
		ui.Textf(c, "%d / %d 个字体面", len(a.rows), a.count(a.scope)).TextColor(c.Theme().TextMuted)
		a.table.Selected = &a.selectedRow
		a.table.Sort = &a.tableSort
		a.table.Key = func(row int) any { return a.rows[row].ID() }
		a.table.CellMenu = func(m *ui.Menu, row, column int) {
			a.selectedRow = row
			a.selectedID = a.rows[row].ID()
			family := a.rows[row].Family
			if m.Item("复制 Family Name").Disabled(family == "").Chosen() {
				mygo.Clipboard.WriteText(family)
			}
		}
		sortTitle := "Name（字体名称）"
		if a.sortMode == 0 {
			sortTitle += " · 原始顺序"
		}
		nameWidth := max(120, (width-32-52-220)*2/3)
		columns := []ui.TableColumn{{Title: "对比", Width: 52}, {Title: sortTitle, ID: "name", Width: nameWidth, Sortable: true}, {Title: "预览（Family Name）", ID: "preview"}}
		listTheme := *c.Theme()
		listTheme.Spacing = 3
		// Visible reports the last frame; reading it requests a new frame when scrolling changes it.
		first, last := a.table.Visible()
		first = min(max(0, first), len(a.rows))
		end := min(max(first, last+1), len(a.rows))
		a.syncPreviews(c, a.rows[first:end], "", a.previewSize, 0, max(44, int(math.Ceil(float64(a.previewSize*1.4)))))
		ui.Themed(c, &listTheme).Grow(1).Children(func() {
			if ui.Table(c, &a.table, columns, len(a.rows), func(row, column int) {
				face := a.rows[row]
				if column == 0 {
					checked := slices.Contains(a.compareIDs, face.ID())
					if ui.Checkbox(c, &checked, "").Changed() {
						if checked && len(a.compareIDs) < 4 {
							a.compareIDs = append(a.compareIDs, face.ID())
							a.compareLimit = false
						} else if checked {
							a.compareLimit = true
						} else {
							a.compareIDs = slices.DeleteFunc(a.compareIDs, func(id string) bool { return id == face.ID() })
							a.compareLimit = false
						}
					}
				} else if column == 1 {
					ui.Text(c, face.FullName).SingleLine().Padding(8, 4)
				} else if face.Family == "" {
					ui.Text(c, "Family Name 未提供").TextColor(c.Theme().TextMuted).SingleLine()
				} else {
					a.previewTextView(c, face, face.Family, a.previewSize, 0, max(44, int(math.Ceil(float64(a.previewSize*1.4))))).Translate(0, -a.previewSize*0.2)
				}
			}).Grow(1).Changed() {
				if a.selectedRow >= 0 && a.selectedRow < len(a.rows) {
					a.selectedID = a.rows[a.selectedRow].ID()
				}
			}
		})
	})
}
