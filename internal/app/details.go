package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fontlens/internal/fonts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
	a.detailKey, a.detailError = "", ""
	a.detailFace = nil
	a.coverageCache, a.coverageLoading = nil, nil
	a.glyphFaceID, a.glyphOutput, a.glyphCharacter = "", "", ""
	a.rawFace, a.rawData = fonts.Entry{}, nil
	a.rawViewOpen = false
}

func (a *appState) ensureDetails(entry fonts.Entry) *fonts.Face {
	key := coverageKey(entry)
	if a.detailKey == key {
		return a.detailFace
	}
	a.releaseDetails()
	a.detailKey = key
	generation := a.detailGeneration
	go func() {
		face, err := fonts.ReadFace(entry.Path, entry.Index)
		a.window.Update(func() {
			if generation != a.detailGeneration || key != a.detailKey {
				return
			}
			if err != nil {
				a.detailError = "字体详情读取失败: " + err.Error()
			} else if coverageKey(face.Entry) != key {
				a.detailError = "字体文件已更改，请刷新列表"
			} else {
				a.detailFace = &face
			}
		})
	}()
	return nil
}

func (a *appState) lookupGlyph(face fonts.Face) {
	a.glyphFaceID = face.ID()
	a.glyphCharacter = ""
	a.glyphOutput = ""
	input := strings.TrimSpace(a.glyphInput)
	if input == "" {
		return
	}
	var glyph uint16
	if strings.HasPrefix(strings.ToLower(input), "gid:") {
		value, err := strconv.ParseUint(strings.TrimSpace(input[4:]), 10, 16)
		if err != nil {
			a.glyphOutput = "字形 ID 应为 0–65535 的整数"
			return
		}
		glyph = uint16(value)
	} else {
		var character rune
		if strings.HasPrefix(strings.ToUpper(input), "U+") {
			value, err := strconv.ParseUint(input[2:], 16, 32)
			if err != nil || !utf8.ValidRune(rune(value)) {
				a.glyphOutput = "请输入有效的 U+ 码位"
				return
			}
			character = rune(value)
		} else {
			runes := []rune(input)
			if len(runes) != 1 {
				a.glyphOutput = "请输入单个字符、U+ 码位或 gid:字形 ID"
				return
			}
			character = runes[0]
		}
		var err error
		glyph, err = fonts.LookupGlyph(face.Path, face.Index, character)
		if err != nil {
			a.glyphOutput = err.Error()
			return
		}
		if glyph == 0 {
			a.glyphOutput = fmt.Sprintf("U+%04X · 未映射到字形", character)
			return
		}
		a.glyphCharacter = string(character)
		a.glyphOutput = fmt.Sprintf("%s · U+%04X · glyph ID %d", string(character), character, glyph)
	}
	if glyph >= face.Glyphs {
		a.glyphOutput = fmt.Sprintf("glyph ID %d 超出字形总数 %d", glyph, face.Glyphs)
		return
	}
	advance, bearing, err := fonts.GlyphMetrics(face.Path, face.Index, glyph)
	if err != nil {
		a.glyphOutput += " · " + err.Error()
		return
	}
	if a.glyphCharacter == "" {
		a.glyphOutput = fmt.Sprintf("glyph ID %d", glyph)
	}
	a.glyphOutput += fmt.Sprintf(" · 水平 advance %d · 左侧 bearing %d（设计单位）", advance, bearing)
}

func (a *appState) details(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "字体详细信息").FontSize(18).Bold()
		if a.selectedRow < 0 || a.selectedRow >= len(a.rows) {
			a.releaseDetails()
			ui.Text(c, "选择字体查看详情").TextColor(c.Theme().TextMuted)
			return
		}
		face := a.ensureDetails(a.rows[a.selectedRow])
		if face == nil {
			if a.detailError != "" {
				ui.Text(c, a.detailError).TextColor(c.Theme().TextMuted)
			} else {
				ui.Text(c, "正在加载字体详情…").TextColor(c.Theme().TextMuted)
			}
			return
		}
		if a.glyphFaceID != face.ID() {
			a.lookupGlyph(*face)
		}
		a.initAxes(*face)
		a.ensureCoverage(*face)
		ui.Scroll(c).Grow(1).Gap(12).Children(func() {
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
			ui.Text(c, "字符与字形查询").Bold()
			if ui.TextInput(c, &a.glyphInput).Placeholder("字符、U+4E00 或 gid:12").Changed() {
				a.lookupGlyph(*face)
			}
			if a.glyphOutput != "" {
				ui.Text(c, a.glyphOutput)
			}
			if a.glyphCharacter != "" {
				a.syncPreviews(c, []fonts.Entry{face.Entry}, a.glyphCharacter, 64, 200, 90)
				a.previewTextView(c, face.Entry, a.glyphCharacter, 64, 200, 90)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "复制字符").Clicked() {
						mygo.Clipboard.WriteText(a.glyphCharacter)
					}
					if ui.Button(c, "复制码位").Clicked() {
						mygo.Clipboard.WriteText(fmt.Sprintf("U+%04X", []rune(a.glyphCharacter)[0]))
					}
				})
			}
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
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "字符集覆盖").Bold().Grow(1)
				if ui.Button(c, "导入字表").Clicked() {
					a.chooseCharset()
				}
				if ui.Button(c, "字符集支持").Clicked() {
					a.coveragePage = true
				}
			})
			if a.charsetMessage != "" {
				ui.Text(c, a.charsetMessage)
			}
			if !face.HasUnicodeCmap() {
				ui.Text(c, "没有可用的 Unicode cmap").TextColor(c.Theme().TextMuted)
			} else if result, ready := a.coverageCache[a.coverageCacheKey(face.Entry)]; ready {
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
			} else {
				ui.Text(c, "正在计算覆盖统计…")
			}
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
