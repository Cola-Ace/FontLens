package main

import (
	"embed"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

//go:embed docs/USER.md docs/PREVIEW_TEXTS.md LICENSE third_party/licenses/*
var embeddedDocs embed.FS

func (a *appState) aboutView(c *ui.Context) {
	guide, _ := embeddedDocs.ReadFile("docs/USER.md")
	presets, _ := embeddedDocs.ReadFile("docs/PREVIEW_TEXTS.md")
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回字体浏览").Clicked() {
				a.about = false
			}
			ui.Textf(c, "字镜 · FontLens %s", mygo.App.Version()).FontSize(20).Bold().Grow(1)
			if ui.Button(c, "使用说明").Clicked() {
				a.aboutTab = 0
			}
			if ui.Button(c, "许可证").Clicked() {
				a.aboutTab = 1
			}
			if ui.Button(c, "预览出处").Clicked() {
				a.aboutTab = 2
			}
		})
		if a.aboutTab == 0 {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).MaxWidth(900).Children(func() {
					markdown.View(c, string(guide), &a.markdown)
				})
			})
			return
		}
		if a.aboutTab == 2 {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).MaxWidth(1100).Children(func() {
					markdown.View(c, string(presets), &a.presetMarkdown)
				})
			})
			return
		}
		names := []string{"LICENSE", "third_party/licenses/README.md", "MyGo-MIT.txt", "purego-Apache-2.0.txt", "go-text-LICENSE.txt", "go-bindings-win32-MIT.txt", "tdewolff-font-MIT.txt", "tdewolff-parse-MIT.txt", "brotli-MIT.txt", "freetype-LICENSE.txt", "freetype-FTL.txt", "modernc-sqlite-BSD-3-Clause.txt", "modernc-sqlite-THIRD-PARTY.md", "SQLite-Public-Domain.txt", "modernc-libc-BSD-3-Clause.txt", "modernc-libc-THIRD-PARTY.md", "Go-BSD-3-Clause.txt", "Unicode-blocks-MIT.txt", "Unicode-LICENSE.txt"}
		ui.SplitPercent(c, &a.licenseSplit, func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					for _, name := range names {
						if ui.Button(c, filepath.Base(name)).Clicked() {
							a.licenseName = name
						}
					}
				})
			})
		}, func() {
			path := a.licenseName
			if path != "LICENSE" && !strings.HasPrefix(path, "third_party/") {
				path = "third_party/licenses/" + path
			}
			data, _ := embeddedDocs.ReadFile(path)
			ui.Scroll(c).Fill().Padding(16).Children(func() {
				ui.Text(c, filepath.Base(path)).FontSize(18).Bold()
				ui.Text(c, string(data)).Font("monospace")
			})
		}).Grow(1)
	})
}
