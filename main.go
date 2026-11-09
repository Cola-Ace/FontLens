package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"fontlens/internal/preview"
	"fontlens/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

//go:embed docs/USER.md LICENSE third_party/licenses/*
var embeddedDocs embed.FS

type settings struct {
	Sources []string `json:"sources"`
}

type appState struct {
	window           *mygo.Window
	tree             ui.OutlineState[string]
	treeRow          int
	table            ui.ListState
	selectedRow      int
	selectedID       string
	leftWidth        float32
	centerWidth      float32
	sources          []string
	folderInput      string
	scope            string
	query            string
	sortMode         int
	faces            []fonts.Face
	faceIndex        map[string]int
	rows             []fonts.Face
	dirs             map[string][]string
	errors           []string
	scanning         bool
	cancelScan       context.CancelFunc
	scanID           int
	showErrors       bool
	format           string
	about            bool
	markdown         markdown.Options
	noticesMarkdown  markdown.Options
	coverageCache    map[string]coverage.Result
	coverageLoading  map[string]bool
	coveragePage     bool
	coverageMode     int
	coverageQuery    string
	coverageSplit    float32
	previewText      string
	previewSize      float32
	previewCache     map[string]*ui.Bitmap
	previewLoading   map[string]bool
	previewErrors    map[string]string
	compareIDs       []string
	comparePage      bool
	compareLimit     bool
	rawViewOpen      bool
	rawFace          fonts.Face
	rawTag           string
	rawData          []byte
	rawError         string
	rawPage          int
	store            *store.Store
	aboutTab         int
	licenseName      string
	licenseSplit     float32
	axisValues       map[string]map[string]float64
	selectedInstance map[string]string
	detailsQuery     string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--self-check" {
		fontPath := ""
		if len(os.Args) > 2 {
			fontPath = os.Args[2]
		}
		if err := selfCheck(fontPath); err != nil {
			log.Fatal(err)
		}
		fmt.Println("FontLens self-check passed")
		return
	}
	app := &appState{leftWidth: 208, centerWidth: 800, coverageSplit: 50, licenseSplit: 25, licenseName: "LICENSE", scope: "all", selectedRow: -1, previewText: "FontLens 123", previewSize: 26}
	app.tree.Open.Add("all")
	app.tree.Open.Add("custom")
	app.loadSettings()
	if path := settingsPath(); path != "" {
		var err error
		app.store, err = store.Open(filepath.Join(filepath.Dir(path), "index.db"))
		if err != nil {
			log.Printf("索引缓存不可用: %v", err)
		}
	}
	mygo.App.WhenReady(func() {
		app.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "字镜 · FontLens", Width: 1440, Height: 900, StateKey: "main",
			Content: ui.View(app.view),
		})
		app.refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if app.store != nil {
		app.store.Close()
	}
}

func (a *appState) readFaces(path string) ([]fonts.Face, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if a.store != nil {
		if faces, found := a.store.Faces(path, info.Size(), info.ModTime().UnixNano()); found {
			return faces, nil
		}
	}
	faces, err := fonts.Read(path)
	if err == nil && a.store != nil {
		_ = a.store.SaveFaces(path, info.Size(), info.ModTime().UnixNano(), faces)
	}
	return faces, err
}

func selfCheck(fontPath string) error {
	if unicode.Version != "17.0.0" {
		return fmt.Errorf("Unicode version: %s", unicode.Version)
	}
	result := coverage.Analyze(map[rune]struct{}{})
	if result.Charsets[0].Total != 6763 || result.Charsets[1].Total != 20923 {
		return fmt.Errorf("charset denominator mismatch")
	}
	foundBlock := false
	for _, row := range result.Blocks {
		if row.Name == "CJK Unified Ideographs" {
			foundBlock = row.Total == 20992
			break
		}
	}
	if !foundBlock {
		return fmt.Errorf("Unicode block denominator mismatch")
	}
	if fontPath != "" {
		return checkFont(fontPath)
	}
	for _, root := range systemSources() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				continue
			}
			path := filepath.Join(root, entry.Name())
			faces, err := fonts.Read(path)
			if err == nil && len(faces) > 0 && faces[0].FullName != "" {
				return checkFont(path)
			}
		}
	}
	return fmt.Errorf("no readable system font")
}

func checkFont(path string) error {
	faces, err := fonts.Read(path)
	if err != nil {
		return err
	}
	if len(faces) == 0 || faces[0].FullName == "" {
		return fmt.Errorf("font has no readable face")
	}
	if len(faces[0].Properties) == 0 {
		return fmt.Errorf("font metadata is empty")
	}
	for _, table := range faces[0].Tables {
		if table.Tag == "fvar" && len(faces[0].Axes) == 0 {
			return fmt.Errorf("variable axes were not parsed")
		}
	}
	bitmap, err := preview.Render(context.Background(), path, 0, nil, "FontLens 123", 28, 260, 52, color.RGBA{R: 35, G: 39, B: 46, A: 255})
	if err != nil {
		return fmt.Errorf("DirectWrite preview: %w", err)
	}
	for _, axis := range faces[0].Axes {
		if axis.Minimum == axis.Maximum {
			continue
		}
		other, renderErr := preview.Render(context.Background(), path, 0, map[string]float32{axis.Tag: axis.Maximum}, "FontLens 123", 28, 260, 52, color.RGBA{R: 35, G: 39, B: 46, A: 255})
		if renderErr != nil {
			return fmt.Errorf("variable preview: %w", renderErr)
		}
		if bytes.Equal(bitmap.Pix, other.Pix) {
			return fmt.Errorf("variable axis %s did not change preview", axis.Tag)
		}
		break
	}
	for pixel := 3; pixel < len(bitmap.Pix); pixel += 4 {
		if bitmap.Pix[pixel] != 0 {
			return checkStore(path, faces)
		}
	}
	return fmt.Errorf("DirectWrite preview is empty")
}

func checkStore(path string, faces []fonts.Face) error {
	dir, err := os.MkdirTemp("", "fontlens-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cache, err := store.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		return err
	}
	defer cache.Close()
	face := faces[0]
	if err := cache.SaveFaces(path, face.Size, face.Modified, faces); err != nil {
		return err
	}
	reloaded, found := cache.Faces(path, face.Size, face.Modified)
	if !found || len(reloaded) != len(faces) || len(reloaded[0].Characters) != len(face.Characters) {
		return fmt.Errorf("font index cache mismatch")
	}
	key := coverageKey(face)
	result := coverage.Analyze(face.Characters)
	if err := cache.SaveCoverage(key, result); err != nil {
		return err
	}
	cached, found := cache.Coverage(key)
	if !found || cached.Codepoints != result.Codepoints || len(cached.Blocks) != len(result.Blocks) {
		return fmt.Errorf("coverage cache mismatch")
	}
	return nil
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
	data, err := json.MarshalIndent(settings{Sources: a.sources}, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0600)
	}
}

func systemSources() []string {
	windows := os.Getenv("WINDIR")
	if windows == "" {
		windows = `C:\Windows`
	}
	roots := []string{filepath.Join(windows, "Fonts")}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		roots = append(roots, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	return roots
}

func (a *appState) refresh() {
	if a.cancelScan != nil {
		a.cancelScan()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelScan = cancel
	a.scanID++
	current := a.scanID
	a.scanning = true
	if a.faceIndex == nil {
		a.faceIndex = make(map[string]int)
	}
	a.dirs = make(map[string][]string)
	a.errors = nil
	roots := append(systemSources(), a.sources...)
	go func() {
		seen := make(map[string]struct{})
		batch := make([]fonts.Face, 0, 32)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			ready := append([]fonts.Face(nil), batch...)
			batch = batch[:0]
			a.window.Update(func() {
				if current != a.scanID {
					return
				}
				for _, face := range ready {
					if index, exists := a.faceIndex[face.ID()]; exists {
						a.faces[index] = face
					} else {
						a.faceIndex[face.ID()] = len(a.faces)
						a.faces = append(a.faces, face)
					}
				}
				a.rebuild()
			})
		}
		for _, root := range roots {
			if ctx.Err() != nil {
				break
			}
			if _, err := os.Stat(root); err != nil {
				continue
			}
			_ = fonts.Scan(ctx, root, a.readFaces, func(path string) {
				if path == root {
					return
				}
				a.window.Update(func() {
					if current != a.scanID {
						return
					}
					parent := filepath.Dir(path)
					if !slices.Contains(a.dirs[parent], path) {
						a.dirs[parent] = append(a.dirs[parent], path)
					}
				})
			}, func(face fonts.Face) {
				id := face.ID()
				if _, exists := seen[id]; exists {
					return
				}
				seen[id] = struct{}{}
				batch = append(batch, face)
				if len(batch) == cap(batch) {
					flush()
				}
			}, func(path string, err error) {
				a.window.Update(func() {
					if current == a.scanID {
						a.errors = append(a.errors, fmt.Sprintf("%s: %v", path, err))
					}
				})
			})
			flush()
		}
		a.window.Update(func() {
			if current == a.scanID {
				if ctx.Err() == nil {
					a.faces = slices.DeleteFunc(a.faces, func(face fonts.Face) bool {
						_, exists := seen[face.ID()]
						return !exists
					})
					a.faceIndex = make(map[string]int, len(a.faces))
					for index, face := range a.faces {
						a.faceIndex[face.ID()] = index
					}
				}
				a.scanning = false
				a.cancelScan = nil
				a.rebuild()
			}
		})
	}()
}

func (a *appState) inScope(face fonts.Face) bool {
	switch a.scope {
	case "all":
		return true
	case "system":
		for _, root := range systemSources() {
			if within(root, face.Path) {
				return true
			}
		}
		return false
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

func (a *appState) detailsMatch(value string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(a.detailsQuery))
}

func (a *appState) rebuild() {
	a.rows = a.rows[:0]
	for _, face := range a.faces {
		if a.inScope(face) && (a.format == "" || face.Format == a.format) && strings.Contains(strings.ToLower(face.FullName), strings.ToLower(a.query)) {
			a.rows = append(a.rows, face)
		}
	}
	if a.sortMode != 0 {
		slices.SortStableFunc(a.rows, func(left, right fonts.Face) int {
			order := strings.Compare(strings.ToLower(left.FullName), strings.ToLower(right.FullName))
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
	}
}

func (a *appState) count(scope string) int {
	n := 0
	for _, face := range a.faces {
		saved := a.scope
		a.scope = scope
		if a.inScope(face) {
			n++
		}
		a.scope = saved
	}
	return n
}

func (a *appState) view(c *ui.Context) {
	if a.about {
		a.aboutView(c)
		return
	}
	if a.comparePage {
		a.compareView(c)
		return
	}
	if a.rawViewOpen {
		a.rawTableView(c)
		return
	}
	if a.coveragePage {
		a.coverageView(c)
		return
	}
	ui.Split(c, &a.leftWidth, func() { a.sidebar(c) }, func() {
		ui.Split(c, &a.centerWidth, func() { a.browser(c) }, func() { a.details(c) }).Fill()
	}).Fill()
}

func (a *appState) openRawTable(face fonts.Face, tag string) {
	a.rawFace, a.rawTag, a.rawData, a.rawError, a.rawPage = face, tag, nil, "", 0
	a.rawViewOpen = true
	go func() {
		data, err := fonts.ReadRawTable(face.Path, face.Index, tag)
		a.window.Update(func() {
			if a.rawFace.ID() != face.ID() || a.rawTag != tag {
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

func (a *appState) compareView(c *ui.Context) {
	faces := make([]fonts.Face, 0, len(a.compareIDs))
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
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.previewText).Placeholder("对比文案").Grow(1)
			if ui.Button(c, "中文").Clicked() {
				a.previewText = "字镜之美 123"
			}
			if ui.Button(c, "英文").Clicked() {
				a.previewText = "FontLens 123"
			}
			if ui.Button(c, "字号 −").Clicked() && a.previewSize > 14 {
				a.previewSize -= 2
			}
			ui.Textf(c, "%.0f px", a.previewSize)
			if ui.Button(c, "字号 +").Clicked() && a.previewSize < 48 {
				a.previewSize += 2
			}
		})
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
							a.previewView(c, face, a.previewSize*2, 450, 180)
						}).Border(1, c.Theme().Border).Radius(8)
					}
				})
			}
		})
	})
}

func coverageKey(face fonts.Face) string {
	return fmt.Sprintf("%s:%d:%d", face.ID(), face.Size, face.Modified)
}

func (a *appState) previewKey(face fonts.Face, size float32, width, height int, ink color.RGBA) string {
	key := fmt.Sprintf("%s:%g:%d:%d:%d:%d:%d:%s", coverageKey(face), size, width, height, ink.R, ink.G, ink.B, a.previewText)
	for _, axis := range face.Axes {
		key += fmt.Sprintf(":%s=%.3f", axis.Tag, a.axisValues[face.ID()][axis.Tag])
	}
	return key
}

func (a *appState) ensurePreview(face fonts.Face, size float32, width, height int, ink color.RGBA) string {
	if a.previewCache == nil {
		a.previewCache = make(map[string]*ui.Bitmap)
		a.previewLoading = make(map[string]bool)
		a.previewErrors = make(map[string]string)
	}
	key := a.previewKey(face, size, width, height, ink)
	if a.previewCache[key] != nil || a.previewLoading[key] || a.previewErrors[key] != "" {
		return key
	}
	a.previewLoading[key] = true
	sample := a.previewText
	axes := make(map[string]float32)
	for tag, value := range a.axisValues[face.ID()] {
		axes[tag] = float32(value)
	}
	go func() {
		bitmap, err := preview.Render(context.Background(), face.Path, face.Index, axes, sample, size, width, height, ink)
		a.window.Update(func() {
			delete(a.previewLoading, key)
			if err != nil {
				a.previewErrors[key] = err.Error()
				return
			}
			if len(a.previewCache) >= 400 {
				a.previewCache = make(map[string]*ui.Bitmap)
			}
			a.previewCache[key] = ui.NewBitmap(bitmap)
		})
	}()
	return key
}

func (a *appState) previewView(c *ui.Context, face fonts.Face, size float32, width, height int) {
	textColor := c.Theme().Text
	key := a.ensurePreview(face, size, width, height, color.RGBA{R: textColor.R, G: textColor.G, B: textColor.B, A: 255})
	if bitmap := a.previewCache[key]; bitmap != nil {
		ui.Image(c, bitmap).Size(float32(width), float32(height)).Fit(ui.Contain)
	} else if message := a.previewErrors[key]; message != "" {
		ui.Text(c, message).TextColor(c.Theme().TextMuted).SingleLine()
	} else {
		ui.Text(c, "预览生成中…").TextColor(c.Theme().TextMuted)
	}
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

func (a *appState) ensureCoverage(face fonts.Face) {
	if face.Characters == nil {
		return
	}
	if a.coverageCache == nil {
		a.coverageCache = make(map[string]coverage.Result)
		a.coverageLoading = make(map[string]bool)
	}
	key := coverageKey(face)
	if _, exists := a.coverageCache[key]; exists || a.coverageLoading[key] {
		return
	}
	a.coverageLoading[key] = true
	go func() {
		result, found := coverage.Result{}, false
		if a.store != nil {
			result, found = a.store.Coverage(key)
		}
		if !found {
			result = coverage.Analyze(face.Characters)
			if a.store != nil {
				_ = a.store.SaveCoverage(key, result)
			}
		}
		a.window.Update(func() {
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
	face := a.rows[a.selectedRow]
	a.ensureCoverage(face)
	result, ready := a.coverageCache[coverageKey(face)]
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			if ui.Button(c, "返回浏览").Clicked() {
				a.coveragePage = false
			}
			ui.Text(c, "字符集支持").FontSize(22).Bold()
		})
		ui.Textf(c, "%s · %s · 面 %d", face.FullName, face.Path, face.Index).SingleLine()
		if !ready {
			ui.Text(c, "正在计算覆盖统计…")
			return
		}
		ui.Textf(c, "Unicode 映射码位 %d · 字形 %d · Unicode 17.0.0", result.Codepoints, face.Glyphs)
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
		ui.Textf(c, "%d / %d · %.2f%% · 缺 %d", row.Covered, row.Total, row.Percent, row.Missing).TextColor(c.Theme().TextMuted)
	})
}

func (a *appState) aboutView(c *ui.Context) {
	guide, _ := embeddedDocs.ReadFile("docs/USER.md")
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
		})
		if a.aboutTab == 0 {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).MaxWidth(900).Children(func() {
					markdown.View(c, string(guide), &a.markdown)
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

func (a *appState) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "字镜").FontSize(24).Bold()
		ui.Text(c, "字体目录").TextColor(c.Theme().TextMuted)
		a.tree.List.Selected = &a.treeRow
		if ui.Outline(c, &a.tree, []string{"all"}, func(node string) []string {
			switch node {
			case "all":
				return []string{"system", "custom"}
			case "custom":
				return a.sources
			default:
				return a.dirs[node]
			}
		}, func(node string) {
			label := node
			switch node {
			case "all":
				label = "全部"
			case "system":
				label = "系统字体"
			case "custom":
				label = "自定义文件夹"
			default:
				label = filepath.Base(node)
			}
			ui.Textf(c, "%s (%d)", label, a.count(node)).SingleLine()
		}).Grow(1).Changed() {
			a.scope = a.tree.Item(a.treeRow)
			a.rebuild()
		}
		ui.TextInput(c, &a.folderInput).Placeholder("路径，或直接点击添加").Label("要添加的字体文件夹")
		if ui.Button(c, "添加文件夹").Clicked() {
			if strings.TrimSpace(a.folderInput) == "" {
				a.chooseFolder()
			} else {
				a.addFolder(a.folderInput)
			}
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
		if slices.Contains(a.sources, a.scope) {
			if ui.Button(c, "移除当前来源").Clicked() {
				a.sources = slices.DeleteFunc(a.sources, func(source string) bool { return source == a.scope })
				a.scope = "custom"
				a.saveSettings()
				a.refresh()
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
		if ui.Button(c, "说明与许可证").Clicked() {
			a.about = true
		}
	})
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
	a.folderInput = ""
	a.saveSettings()
	a.refresh()
}

func (a *appState) browser(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.SearchField(c, &a.query).Placeholder("搜索字体全名").Grow(1).Changed() {
				a.rebuild()
			}
			if ui.Button(c, fmt.Sprintf("对比 (%d)", len(a.compareIDs))).Clicked() && len(a.compareIDs) >= 2 {
				a.comparePage = true
			}
			labels := []string{"默认排序", "名称 ↑", "名称 ↓"}
			if ui.Button(c, labels[a.sortMode]).Clicked() {
				a.sortMode = (a.sortMode + 1) % 3
				a.rebuild()
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
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.previewText).Placeholder("预览文案").Grow(1)
			if ui.Button(c, "中文").Clicked() {
				a.previewText = "字镜之美 123"
			}
			if ui.Button(c, "英文").Clicked() {
				a.previewText = "FontLens 123"
			}
			if ui.Button(c, "字号 −").Clicked() && a.previewSize > 14 {
				a.previewSize -= 2
			}
			ui.Textf(c, "%.0f px", a.previewSize)
			if ui.Button(c, "字号 +").Clicked() && a.previewSize < 48 {
				a.previewSize += 2
			}
		})
		ui.Textf(c, "%d / %d 个字体面", len(a.rows), a.count(a.scope)).TextColor(c.Theme().TextMuted)
		a.table.Selected = &a.selectedRow
		a.table.Key = func(row int) any { return a.rows[row].ID() }
		columns := []ui.TableColumn{{Title: "对比", Width: 52}, {Title: "Name（字体名称）"}, {Title: "预览", Width: 220}}
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
			} else {
				a.previewView(c, face, a.previewSize, 200, 44)
			}
		}).Grow(1).Changed() {
			if a.selectedRow >= 0 && a.selectedRow < len(a.rows) {
				a.selectedID = a.rows[a.selectedRow].ID()
			}
		}
	})
}

func (a *appState) details(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "字体详细信息").FontSize(18).Bold()
		if a.selectedRow < 0 || a.selectedRow >= len(a.rows) {
			ui.Text(c, "选择字体查看详情").TextColor(c.Theme().TextMuted)
			return
		}
		face := a.rows[a.selectedRow]
		a.initAxes(face)
		a.ensureCoverage(face)
		ui.Scroll(c).Grow(1).Gap(12).Children(func() {
			ui.Text(c, face.FullName).FontSize(24).Bold()
			ui.Text(c, face.Path)
			ui.Textf(c, "面 %d / %d · %s", face.Index+1, face.Count, face.Format)
			ui.Textf(c, "文件大小 %d bytes · 文件修改 %s", face.Size, time.Unix(0, face.Modified).Local().Format("2006-01-02 15:04:05"))
			ui.Text(c, "字体大样").Bold()
			a.previewView(c, face, a.previewSize*2, 380, 150)
			ui.Divider(c)
			ui.Textf(c, "Family: %s", face.Family)
			ui.Textf(c, "Subfamily: %s", face.Subfamily)
			ui.Textf(c, "PostScript: %s", face.PostScript)
			ui.Textf(c, "字形数: %d · Unicode 映射码位: %d", face.Glyphs, len(face.Characters))
			ui.Textf(c, "采用 cmap: %s", face.Cmap)
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
				if ui.Button(c, "字符集支持").Clicked() {
					a.coveragePage = true
				}
			})
			if face.Characters == nil {
				ui.Text(c, "没有可用的 Unicode cmap").TextColor(c.Theme().TextMuted)
			} else if result, ready := a.coverageCache[coverageKey(face)]; ready {
				labels := []string{"字表", "Unicode 区块"}
				if ui.Button(c, labels[a.coverageMode]).Clicked() {
					a.coverageMode = (a.coverageMode + 1) % 2
				}
				ui.SearchField(c, &a.coverageQuery).Placeholder("搜索统计项")
				rows := result.Charsets
				if a.coverageMode == 1 {
					rows = result.Blocks
				}
				filtered := make([]coverage.Row, 0, len(rows))
				for _, row := range rows {
					if strings.Contains(strings.ToLower(row.Name), strings.ToLower(a.coverageQuery)) {
						filtered = append(filtered, row)
					}
				}
				ui.List(c, nil, len(filtered), func(index int) { coverageRow(c, filtered[index]) }).Height(220)
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
				line := fmt.Sprintf("ID %d · %d/%d/%04X: %s", name.ID, name.Platform, name.Encoding, name.Language, name.Value)
				if a.detailsMatch(line) {
					ui.Text(c, line)
				}
			}
			ui.Text(c, "表目录").Bold()
			for _, table := range face.Tables {
				label := fmt.Sprintf("%s · %d bytes", table.Tag, table.Size)
				if a.detailsMatch(label) && ui.Button(c, label).Clicked() {
					a.openRawTable(face, table.Tag)
				}
			}
		})
	})
}
