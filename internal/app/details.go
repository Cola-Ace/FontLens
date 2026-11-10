package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"github.com/egoist/mygo/ui"
)

type detailResult struct {
	face        fonts.Face
	coverage    coverage.Result
	coverageKey string
}

func (a *appState) detailsMatch(value string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(a.detailsQuery))
}

func (a *appState) initAxes(face fonts.Face) {
	if len(face.Axes) == 0 {
		return
	}
	if a.axisValues == nil {
		a.axisValues = make(map[string]map[string]float64)
	}
	if a.axisValues[face.ID()] == nil {
		values := make(map[string]float64)
		for _, axis := range face.Axes {
			values[axis.Tag] = float64(axis.Default)
		}
		a.axisValues[face.ID()] = values
	}
}

func (a *appState) releaseDetails() {
	if a.detailKey == "" {
		return
	}
	a.detailGeneration++
	a.detailLoading.finish()
	a.detailLoading = nil
	a.detailKey, a.detailError = "", ""
	a.detailFace = nil
	a.detailPending = nil
	for _, hint := range a.coverageLoading {
		hint.finish()
	}
	a.coverageCache, a.coverageLoading = nil, nil
	a.rawLoading.finish()
	a.rawFace, a.rawData = fonts.Entry{}, nil
	a.rawViewOpen = false
}

func (a *appState) ensureDetails(entry fonts.Entry) *fonts.Face {
	key := coverageKey(entry)
	if a.detailKey == key {
		return a.detailFace
	}
	a.detailGeneration++
	a.detailPending = nil
	a.detailLoading.finish()
	a.detailLoading = newLoadingHint(a.window)
	for _, hint := range a.coverageLoading {
		hint.finish()
	}
	a.detailError = ""
	a.rawLoading.finish()
	a.rawFace, a.rawData = fonts.Entry{}, nil
	a.rawViewOpen = false
	a.detailKey = key
	generation := a.detailGeneration
	cacheKey := a.coverageCacheKey(entry)
	imported := slices.Clone(a.charsets)
	go func() {
		face, err := fonts.ReadFace(entry.Path, entry.Index)
		var result coverage.Result
		if err == nil && coverageKey(face.Entry) == key && face.HasUnicodeCmap() {
			result = a.readCoverage(face, cacheKey, imported)
		}
		a.window.Update(func() {
			if generation != a.detailGeneration || key != a.detailKey {
				return
			}
			if err != nil {
				a.detailFace = nil
				a.detailError = "字体详情读取失败: " + err.Error()
			} else if coverageKey(face.Entry) != key {
				a.detailFace = nil
				a.detailError = "字体文件已更改，请刷新列表"
			} else {
				a.detailPending = &detailResult{face: face, coverage: result, coverageKey: cacheKey}
				return
			}
			a.detailLoading.finish()
			a.detailLoading = nil
		})
	}()
	return a.detailFace
}

func (a *appState) commitDetails() {
	result := a.detailPending
	a.detailFace = &result.face
	a.detailPending = nil
	a.detailLoading.finish()
	a.detailLoading = nil
	for _, hint := range a.coverageLoading {
		hint.finish()
	}
	a.coverageCache = make(map[string]coverage.Result)
	a.coverageLoading = make(map[string]*loadingHint)
	if result.face.HasUnicodeCmap() && result.coverageKey == a.coverageCacheKey(result.face.Entry) {
		a.coverageCache[result.coverageKey] = result.coverage
	}
}

func (a *appState) details(c *ui.Context) {
	panel := ui.Column(c).Fill().Padding(16).Gap(12)
	width := panel.LayoutBounds().W - 32
	if width <= 0 {
		width = defaultDetailWidth - 32
	}
	previewWidth := int(width)
	panel.Children(func() {
		if a.selectedRow < 0 || a.selectedRow >= len(a.rows) {
			a.releaseDetails()
			ui.Text(c, "字体详细信息").FontSize(18).Bold()
			ui.Text(c, "选择字体查看详情").TextColor(c.Theme().TextMuted)
			return
		}
		face := a.ensureDetails(a.rows[a.selectedRow])
		if pending := a.detailPending; pending != nil {
			a.initAxes(pending.face)
			key := a.syncPreview(c, pending.face.Entry, a.previewText, a.previewSize, previewWidth, 0)
			// Publish the new font's text and preview together, including render errors.
			if a.previewBitmaps[key] != nil || a.previewErrors[key] != "" {
				a.commitDetails()
				face = a.detailFace
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字体详细信息").FontSize(18).Bold().Grow(1)
			if a.detailLoading.show() {
				ui.Text(c, "加载中…").FontSize(12).TextColor(c.Theme().TextMuted)
			}
		})
		if face == nil {
			if a.detailError != "" {
				ui.Text(c, a.detailError).TextColor(c.Theme().TextMuted)
			}
			return
		}
		a.initAxes(*face)
		if a.detailLoading == nil {
			a.ensureCoverage(*face)
		}
		ui.Scroll(c).Grow(1).Gap(12).Disabled(a.detailLoading != nil).DimDisabled(false).Children(func() {
			ui.Text(c, face.FullName).FontSize(24).Bold()
			ui.Text(c, face.Path)
			ui.Textf(c, "面 %d / %d · %s", face.Index+1, face.Count, face.Format)
			ui.Textf(c, "文件大小 %d bytes · 文件修改 %s", face.Size, time.Unix(0, face.Modified).Local().Format("2006-01-02 15:04:05"))
			ui.Divider(c)
			ui.Textf(c, "Family Name: %s", face.Family)
			ui.Textf(c, "Subfamily: %s", face.Subfamily)
			ui.Textf(c, "PostScript: %s", face.PostScript)
			ui.Textf(c, "字形数: %d · Unicode 映射码位: %d", face.Glyphs, face.Characters.Count())
			ui.Textf(c, "采用 cmap: %s", face.Cmap)
			ui.Divider(c)
			ui.Column(c).Children(func() {
				a.syncPreview(c, face.Entry, a.previewText, a.previewSize, previewWidth, 0)
				a.previewView(c, face.Entry, a.previewSize, previewWidth, 0)
			})
			ui.Divider(c)
			if len(face.Axes) > 0 {
				ui.Text(c, "可变字体轴").Bold()
				if len(face.Instances) > 0 {
					if a.selectedInstance == nil {
						a.selectedInstance = make(map[string]string)
					}
					selected := a.selectedInstance[face.ID()]
					if selected == "" {
						selected = "选择命名实例"
					}
					names := make([]string, 0, len(face.Instances))
					for _, instance := range face.Instances {
						names = append(names, instance.Name)
					}
					if ui.Select(c, &selected, names).Changed() {
						a.selectedInstance[face.ID()] = selected
						for _, instance := range face.Instances {
							if instance.Name == selected {
								for tag, value := range instance.Coordinates {
									a.axisValues[face.ID()][tag] = float64(value)
								}
								break
							}
						}
					}
				}
				for _, axis := range face.Axes {
					name := axis.Name
					if name == "" {
						name = axis.Tag
					}
					ui.Textf(c, "%s (%s) · %.1f–%.1f", name, axis.Tag, axis.Minimum, axis.Maximum)
					value := a.axisValues[face.ID()][axis.Tag]
					ui.Row(c).Gap(8).Children(func() {
						if ui.Slider(c, &value, float64(axis.Minimum), float64(axis.Maximum)).Grow(1).Changed() {
							a.axisValues[face.ID()][axis.Tag] = value
							a.selectedInstance[face.ID()] = ""
						}
						if ui.NumberInput(c, &value, float64(axis.Minimum), float64(axis.Maximum), 1).Width(90).Changed() {
							a.axisValues[face.ID()][axis.Tag] = value
							a.selectedInstance[face.ID()] = ""
						}
					})
				}
				if ui.Button(c, "恢复默认轴值").Clicked() {
					for _, axis := range face.Axes {
						a.axisValues[face.ID()][axis.Tag] = float64(axis.Default)
					}
					a.selectedInstance[face.ID()] = ""
				}
				ui.Divider(c)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "字符集覆盖").Bold().Grow(1)
				if a.coverageLoading[a.coverageCacheKey(face.Entry)].show() {
					ui.Text(c, "更新中…").TextColor(c.Theme().TextMuted)
				}
				if ui.Button(c, "字符集支持").Clicked() {
					a.coveragePage = true
				}
			})
			if !face.HasUnicodeCmap() {
				ui.Text(c, "没有可用的 Unicode cmap").TextColor(c.Theme().TextMuted)
			} else if result, ready := a.cachedCoverage(face.Entry); ready {
				labels := []string{"字表", "Unicode 区块"}
				if ui.Button(c, labels[a.coverageMode]).Clicked() {
					a.coverageMode = (a.coverageMode + 1) % 2
				}
				ui.SearchField(c, &a.coverageQuery).Placeholder("搜索统计项")
				rows := result.Charsets
				if a.coverageMode == 1 {
					rows = result.Blocks
				}
				for _, row := range rows {
					if strings.Contains(strings.ToLower(row.Name), strings.ToLower(a.coverageQuery)) {
						coverageRow(c, row)
					}
				}
			} else if a.coverageLoading[a.coverageCacheKey(face.Entry)].show() {
				ui.Text(c, "正在计算覆盖统计…")
			}
			ui.Divider(c)
			ui.SearchField(c, &a.detailsQuery).Placeholder("搜索名称记录、属性或表")
			ui.Text(c, "字体属性").Bold()
			group := ""
			for _, property := range face.Properties {
				line := fmt.Sprintf("%s: %s", property.Name, property.Value)
				if !a.detailsMatch(property.Group + " " + line) {
					continue
				}
				if property.Group != group {
					group = property.Group
					ui.Text(c, group).Bold()
				}
				ui.Text(c, line)
			}
			ui.Divider(c)
			ui.Text(c, "名称记录").Bold()
			for _, name := range face.Names {
				language := fmt.Sprintf("%04X", name.Language)
				if name.LanguageTag != "" {
					language += " " + name.LanguageTag
				}
				value := name.Value
				if value == "" {
					value = "（空字符串）"
				}
				line := fmt.Sprintf("ID %d · %d/%d/%s: %s", name.ID, name.Platform, name.Encoding, language, value)
				if a.detailsMatch(line) {
					ui.Text(c, line)
				}
			}
			ui.Divider(c)
			ui.Text(c, "表目录").Bold()
			offsetLabel := "文件偏移"
			if face.Format == "WOFF" || face.Format == "WOFF2" {
				offsetLabel = "解包后偏移"
			}
			for _, table := range face.Tables {
				label := fmt.Sprintf("%s · %d bytes · %s 0x%X · 目录 checksum 0x%08X", table.Tag, table.Size, offsetLabel, table.Offset, table.Checksum)
				if a.detailsMatch(label) && ui.Button(c, label).Clicked() {
					a.openRawTable(face.Entry, table.Tag)
				}
			}
		})
	})
}
