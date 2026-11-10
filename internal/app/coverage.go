package app

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func coverageKey(face fonts.Entry) string {
	return fmt.Sprintf("%s:%d:%d", face.ID(), face.Size, face.Modified)
}

func (a *appState) coverageCacheKey(face fonts.Entry) string {
	return fmt.Sprintf("%s:charsets:%d", coverageKey(face), a.charsetRevision)
}

func (a *appState) ensureCoverage(face fonts.Face) {
	if !face.HasUnicodeCmap() {
		return
	}
	if a.coverageCache == nil {
		a.coverageCache = make(map[string]coverage.Result)
		a.coverageLoading = make(map[string]bool)
	}
	key := a.coverageCacheKey(face.Entry)
	if _, exists := a.coverageCache[key]; exists || a.coverageLoading[key] {
		return
	}
	a.coverageLoading[key] = true
	imported := slices.Clone(a.charsets)
	generation := a.detailGeneration
	entry, characters := face.Entry, face.Characters
	go func() {
		result, found := coverage.Result{}, false
		if a.store != nil {
			result, found = a.store.Coverage(key)
		}
		if !found {
			result = coverage.AnalyzeWith(characters, imported)
			if a.store != nil {
				_ = a.store.SaveCoverage(key, result)
			}
		}
		a.window.Update(func() {
			if generation != a.detailGeneration || key != a.coverageCacheKey(entry) {
				return
			}
			a.coverageCache[key] = result
			delete(a.coverageLoading, key)
		})
	}()
}

func (a *appState) coverageView(c *ui.Context) {
	if a.selectedRow < 0 || a.selectedRow >= len(a.rows) {
		a.coveragePage = false
		return
	}
	face := a.ensureDetails(a.rows[a.selectedRow])
	if face == nil {
		a.coveragePage = false
		return
	}
	a.ensureCoverage(*face)
	result, ready := a.coverageCache[a.coverageCacheKey(face.Entry)]
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回浏览").Clicked() {
				a.coveragePage = false
			}
			ui.Text(c, "字符集支持").FontSize(22).Bold()
		})
		ui.Textf(c, "%s · %s · 面 %d", face.FullName, face.Path, face.Index).SingleLine()
		if !face.HasUnicodeCmap() {
			ui.Text(c, "没有可用的 Unicode cmap，无法计算字符集覆盖").TextColor(c.Theme().TextMuted)
			return
		}
		if !ready {
			ui.Text(c, "正在计算覆盖统计…")
			return
		}
		ui.Textf(c, "Unicode 映射码位 %d · 字形 %d · Unicode 17.0.0", result.Codepoints, face.Glyphs)
		if ui.Button(c, "导入 UTF-8 字表").Clicked() {
			a.chooseCharset()
		}
		if a.charsetMessage != "" {
			ui.Text(c, a.charsetMessage)
		}
		ui.SearchField(c, &a.coverageQuery).Placeholder("搜索字表或 Unicode 区块")
		ui.SplitPercent(c, &a.coverageSplit, func() {
			a.coverageList(c, "字表", result.Charsets)
		}, func() {
			a.coverageList(c, "Unicode 区块", result.Blocks)
		}).Grow(1)
	})
}

func (a *appState) coverageList(c *ui.Context, title string, rows []coverage.Row) {
	filtered := make([]coverage.Row, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), strings.ToLower(a.coverageQuery)) {
			filtered = append(filtered, row)
		}
	}
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Textf(c, "%s (%d)", title, len(filtered)).Bold()
		ui.List(c, nil, len(filtered), func(index int) { coverageRow(c, filtered[index]) }).Grow(1)
	})
}

func coverageRow(c *ui.Context, row coverage.Row) {
	ui.Column(c).Padding(8, 4).Gap(2).Children(func() {
		ui.Text(c, row.Name).SingleLine()
		if row.Pending {
			ui.Text(c, "待导入 · 可在“字符集支持”页导入字表").TextColor(c.Theme().TextMuted)
			return
		}
		ui.Textf(c, "%d / %d · %.2f%% · 缺 %d", row.Covered, row.Total, row.Percent, row.Missing).TextColor(c.Theme().TextMuted)
	})
}

func (a *appState) chooseCharset() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "导入 UTF-8 字表"})
		if err != nil {
			a.window.Update(func() { a.charsetMessage = err.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		path := paths[0]
		data, err := os.ReadFile(path)
		if err != nil {
			a.window.Update(func() { a.charsetMessage = fmt.Sprintf("%s: %v", path, err) })
			return
		}
		definition, err := coverage.Parse(path, data)
		a.window.Update(func() {
			if err != nil {
				a.charsetMessage = fmt.Sprintf("%s: %v", path, err)
				return
			}
			a.charsets = append(slices.DeleteFunc(a.charsets, func(item coverage.Definition) bool { return item.Source == path }), definition)
			a.charsetRevision++
			a.saveSettings()
			a.charsetMessage = fmt.Sprintf("已导入 %s · %d 字", definition.Name, len([]rune(definition.Members)))
		})
	}()
}
