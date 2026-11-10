package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"fontlens/internal/coverage"
)

type settings struct {
	Sources           []string              `json:"sources"`
	Scope             string                `json:"scope"`
	Open              []string              `json:"open"`
	Charsets          []coverage.Definition `json:"charsets"`
	CharsetRevision   int64                 `json:"charsetRevision"`
	ThemeMode         int                   `json:"themeMode"`
	LeftWidth         float32               `json:"leftWidth,omitempty"`
	CenterWidth       float32               `json:"centerWidth,omitempty"`
	CoverageSplit     float32               `json:"coverageSplit,omitempty"`
	TableWidths       map[string]float32    `json:"tableWidths,omitempty"`
	PreviewPreset     string                `json:"previewPreset"`
	CustomPreviewText string                `json:"customPreviewText"`
}

func settingsPath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "FontLens", "settings.json")
}

func (a *appState) loadSettings() {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return
	}
	var saved settings
	if json.Unmarshal(data, &saved) == nil {
		a.sources = saved.Sources
		if saved.Scope != "" {
			a.scope = saved.Scope
		}
		if saved.Open != nil {
			a.tree.Open.Clear()
			for _, node := range saved.Open {
				a.tree.Open.Add(node)
			}
		}
		a.charsets = saved.Charsets
		a.charsetRevision = saved.CharsetRevision
		if saved.PreviewPreset != "" {
			a.customPreviewText = saved.CustomPreviewText
			a.selectPreviewPreset(saved.PreviewPreset)
		}
		if saved.ThemeMode >= 0 && saved.ThemeMode <= 2 {
			a.themeMode = saved.ThemeMode
		}
		if saved.LeftWidth != 208 || saved.CenterWidth != 800 {
			if saved.LeftWidth > 0 {
				a.leftWidth = saved.LeftWidth
			}
			if saved.CenterWidth > 0 {
				a.centerWidth = saved.CenterWidth
				if saved.LeftWidth == float32(defaultLeftWidth) && saved.CenterWidth == 874 {
					a.centerWidth = defaultCenterWidth
				}
			}
		}
		if saved.CoverageSplit > 0 && saved.CoverageSplit < 100 {
			a.coverageSplit = saved.CoverageSplit
		}
		for id, width := range saved.TableWidths {
			if width > 0 {
				if a.table.Columns.Widths == nil {
					a.table.Columns.Widths = make(map[string]float32)
				}
				a.table.Columns.Widths[id] = width
			}
		}
	}
}

func (a *appState) saveSettings() {
	path := settingsPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	open := make([]string, 0, a.tree.Open.Len())
	for node := range a.tree.Open.All() {
		open = append(open, node)
	}
	slices.Sort(open)
	data, err := json.MarshalIndent(settings{
		Sources: a.sources, Scope: a.scope, Open: open, Charsets: a.charsets,
		CharsetRevision: a.charsetRevision, ThemeMode: a.themeMode,
		LeftWidth: a.leftWidth, CenterWidth: a.centerWidth,
		CoverageSplit: a.coverageSplit,
		TableWidths:   a.table.Columns.Widths,
		PreviewPreset: a.previewPreset, CustomPreviewText: a.customPreviewText,
	}, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0600)
	}
}
