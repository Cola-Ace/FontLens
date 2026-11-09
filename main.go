package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"fontlens/internal/preview"
	"fontlens/internal/store"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/hidpi"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

//go:embed docs/USER.md docs/PREVIEW_TEXTS.md LICENSE third_party/licenses/*
var embeddedDocs embed.FS

type settings struct {
	Sources         []string              `json:"sources"`
	Scope           string                `json:"scope"`
	Open            []string              `json:"open"`
	Charsets        []coverage.Definition `json:"charsets"`
	CharsetRevision int64                 `json:"charsetRevision"`
	ThemeMode       int                   `json:"themeMode"`
}

type appState struct {
	window            *mygo.Window
	tree              ui.OutlineState[string]
	treeRow           int
	table             ui.ListState
	selectedRow       int
	selectedID        string
	detailFace        *fonts.Face
	detailKey         string
	detailGeneration  int
	detailError       string
	leftWidth         float32
	centerWidth       float32
	sources           []string
	charsets          []coverage.Definition
	charsetRevision   int64
	charsetMessage    string
	themeMode         int
	scope             string
	query             string
	sortMode          int
	tableSort         ui.SortOrder
	sortSeen          ui.SortOrder
	faces             []fonts.Entry
	faceIndex         map[string]int
	systemIDs         map[string]bool
	scopeCounts       map[string]map[string]struct{}
	rows              []fonts.Entry
	dirs              map[string][]string
	errors            []string
	scanning          bool
	cancelScan        context.CancelFunc
	scanID            int
	showErrors        bool
	format            string
	about             bool
	markdown          markdown.Options
	presetMarkdown    markdown.Options
	noticesMarkdown   markdown.Options
	coverageCache     map[string]coverage.Result
	coverageLoading   map[string]bool
	coveragePage      bool
	coverageMode      int
	coverageQuery     string
	coverageSplit     float32
	previewText       string
	previewPreset     string
	previewSize       float32
	previewLineHeight float32
	previewCache      map[string]*ui.Bitmap
	previewLast       map[string]*ui.Bitmap
	previewLoading    map[string]bool
	previewErrors     map[string]string
	compareIDs        []string
	comparePage       bool
	compareLimit      bool
	rawViewOpen       bool
	rawFace           fonts.Entry
	rawTag            string
	rawData           []byte
	rawError          string
	rawPage           int
	store             *store.Store
	aboutTab          int
	licenseName       string
	licenseSplit      float32
	axisValues        map[string]map[string]float64
	selectedInstance  map[string]string
	detailsQuery      string
	glyphInput        string
	glyphOutput       string
	glyphFaceID       string
	glyphCharacter    string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(mygo.App.Version())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--self-check" {
		fontPath := ""
		sample := "FontLens 123"
		if len(os.Args) > 2 {
			fontPath = os.Args[2]
		}
		if len(os.Args) > 3 {
			sample = os.Args[3]
		}
		if err := selfCheck(fontPath, sample); err != nil {
			log.Fatal(err)
		}
		fmt.Println("FontLens self-check passed")
		return
	}
	app := &appState{leftWidth: 208, centerWidth: 800, coverageSplit: 50, licenseSplit: 25, licenseName: "LICENSE", scope: "all", selectedRow: -1, previewText: "FontLens 123", previewPreset: "自定义", previewSize: 30, previewLineHeight: 1.5}
	app.sortMode = 1
	app.tableSort = ui.SortOrder{Column: "name"}
	app.sortSeen = app.tableSort
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
			Title: "字镜 · FontLens", Width: 1440, Height: 900, MinWidth: 1024, MinHeight: 720, StateKey: "main",
			Content: ui.View(app.view),
		})
		app.refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	app.saveSettings()
	if app.store != nil {
		app.store.Close()
	}
}

func (a *appState) readFaces(path string) ([]fonts.Entry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if a.store != nil {
		if faces, found := a.store.Faces(path, info.Size(), info.ModTime().UnixNano()); found {
			return faces, nil
		}
	}
	faces, err := fonts.ReadIndex(path)
	if err == nil && a.store != nil {
		_ = a.store.SaveFaces(path, info.Size(), info.ModTime().UnixNano(), faces)
	}
	return faces, err
}

func selfCheck(fontPath, sample string) error {
	if err := checkScopeCounts(); err != nil {
		return err
	}
	if err := checkNameSorting(); err != nil {
		return err
	}
	if err := checkFontDeduplication(); err != nil {
		return err
	}
	imported, err := coverage.Parse("sample.txt", []byte("---\nname: 示例\ncount: 2\n---\n字\n字\n镜\n"))
	if err != nil || imported.Name != "示例" || len([]rune(imported.Members)) != 2 {
		return fmt.Errorf("charset import check failed: %v", err)
	}
	if _, err := coverage.Parse("invalid.txt", []byte("---\ncount: 3\n---\n字\n镜\n")); err == nil {
		return fmt.Errorf("charset count validation failed")
	}
	if unicode.Version != "17.0.0" {
		return fmt.Errorf("Unicode version: %s", unicode.Version)
	}
	result := coverage.Analyze(nil)
	if result.Charsets[0].Total != 6763 || result.Charsets[1].Total != 20923 {
		return fmt.Errorf("charset denominator mismatch")
	}
	if !result.Charsets[2].Pending || result.Charsets[2].Total != 0 {
		return fmt.Errorf("pending charset status mismatch")
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
		return checkFont(fontPath, sample)
	}
	systemFaces := 0
	if err := fonts.SystemFaces(context.Background(), func(string, int) { systemFaces++ }, func(error) {}); err != nil {
		return fmt.Errorf("DirectWrite system enumeration: %w", err)
	}
	if systemFaces == 0 {
		return fmt.Errorf("DirectWrite returned no local system fonts")
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
				return checkFont(path, sample)
			}
		}
	}
	return fmt.Errorf("no readable system font")
}

func checkScopeCounts() error {
	root := filepath.Join(os.TempDir(), "fontlens-scope-check")
	child := filepath.Join(root, "nested")
	collection := filepath.Join(child, "faces.ttc")
	app := appState{
		sources: []string{root, child},
		faces: []fonts.Entry{
			{Path: collection, Index: 0, FullName: "Font Regular"},
			{Path: collection, Index: 1, FullName: "Font Bold"},
			{Path: filepath.Join(root, "single.ttf"), FullName: " font regular "},
			{Path: filepath.Join(os.TempDir(), "outside.ttf"), FullName: "Font Italic"},
		},
	}
	app.systemIDs = map[string]bool{app.faces[0].ID(): true, app.faces[1].ID(): true}
	app.recount()
	if app.count("all") != 3 || app.count("system") != 2 || app.count("custom") != 2 || app.count(root) != 2 || app.count(child) != 2 {
		return fmt.Errorf("directory face counts mismatch")
	}
	return nil
}

func checkNameSorting() error {
	app := appState{scope: "all", faces: []fonts.Entry{
		{Path: "font10.ttf", FullName: "Font 10"},
		{Path: "font2.ttf", FullName: "font 2"},
	}}
	app.sortMode = 1
	app.rebuild()
	if app.rows[0].FullName != "font 2" {
		return fmt.Errorf("natural name ascending sort failed")
	}
	app.sortMode = 2
	app.rebuild()
	if app.rows[0].FullName != "Font 10" {
		return fmt.Errorf("natural name descending sort failed")
	}
	return nil
}

func checkFontDeduplication() error {
	root := filepath.Join(os.TempDir(), "fontlens-dedup-check")
	app := appState{scope: "all", faces: []fonts.Entry{
		{Path: filepath.Join(root, "original.ttf"), Format: "TTF", FullName: "Font Regular"},
		{Path: filepath.Join(root, "copies", "copy.woff2"), Format: "WOFF2", FullName: " font regular "},
		{Path: filepath.Join(root, "collection.ttc"), Index: 1, Format: "TTC", FullName: "Font Bold"},
		{Path: filepath.Join(root, "unnamed-a.ttf")},
		{Path: filepath.Join(root, "unnamed-b.ttf")},
	}}
	app.selectedID = app.faces[0].ID()
	for _, mode := range []int{0, 1, 2} {
		app.sortMode = mode
		app.rebuild()
		if len(app.rows) != 4 || app.selectedRow < 0 || app.rows[app.selectedRow].ID() != app.faces[0].ID() {
			return fmt.Errorf("font deduplication or selection failed in sort mode %d", mode)
		}
	}
	app.format = "WOFF2"
	app.query = "regular"
	app.rebuild()
	if len(app.rows) != 1 || app.rows[0].ID() != app.faces[1].ID() {
		return fmt.Errorf("font deduplication format filter failed")
	}
	app.format = ""
	app.scope = filepath.Join(root, "copies")
	app.rebuild()
	if len(app.rows) != 1 || app.rows[0].ID() != app.faces[1].ID() {
		return fmt.Errorf("font deduplication directory filter failed")
	}
	return nil
}

func checkFont(path, sample string) error {
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
	container, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if faces[0].Format == "WOFF" || faces[0].Format == "WOFF2" {
		container, err = fonts.DecodeWebFont(path)
		if err != nil {
			return err
		}
	}
	for _, face := range faces {
		for _, table := range face.Tables {
			if table.Tag != "head" {
				continue
			}
			end := uint64(table.Offset) + uint64(table.Size)
			if end > uint64(len(container)) {
				return fmt.Errorf("face %d head offset is out of range", face.Index)
			}
			raw, readErr := fonts.ReadRawTable(path, face.Index, table.Tag)
			if readErr != nil || !bytes.Equal(raw, container[table.Offset:end]) {
				return fmt.Errorf("face %d head directory offset mismatch: %v", face.Index, readErr)
			}
			break
		}
	}
	for _, table := range faces[0].Tables {
		if table.Tag == "fvar" && len(faces[0].Axes) == 0 {
			return fmt.Errorf("variable axes were not parsed")
		}
	}
	for character := range faces[0].Characters.All() {
		glyph, lookupErr := fonts.LookupGlyph(path, 0, character)
		if lookupErr != nil || glyph == 0 || glyph >= faces[0].Glyphs {
			return fmt.Errorf("glyph lookup U+%04X: glyph %d: %v", character, glyph, lookupErr)
		}
		if _, _, lookupErr = fonts.GlyphMetrics(path, 0, glyph); lookupErr != nil {
			return fmt.Errorf("glyph metrics: %w", lookupErr)
		}
		break
	}
	bitmap, err := preview.Render(context.Background(), path, 0, nil, sample, 28, 1.4, 260, 52, 1, color.RGBA{R: 35, G: 39, B: 46, A: 255})
	if err != nil {
		return fmt.Errorf("DirectWrite preview: %w", err)
	}
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		wrapped, err := preview.Render(context.Background(), path, 0, nil, strings.Repeat(sample+" ", 8), 28, 1.4, 260, 150, scale, color.RGBA{R: 35, G: 39, B: 46, A: 255})
		if err != nil {
			return fmt.Errorf("wrapped preview at %g×: %w", scale, err)
		}
		if wrapped.Rect.Dx() != int(math.Ceil(float64(260*scale))) || wrapped.Rect.Dy() != int(math.Ceil(float64(150*scale))) {
			return fmt.Errorf("preview pixel dimensions at %g× mismatch", scale)
		}
		secondLine, antialiased := false, false
		for y := 0; y < wrapped.Rect.Dy(); y++ {
			for x := 0; x < wrapped.Rect.Dx(); x++ {
				alpha := wrapped.RGBAAt(x, y).A
				if alpha > 0 && y >= int(65*scale) {
					secondLine = true
				}
				if alpha > 0 && alpha < 255 {
					antialiased = true
				}
			}
		}
		if !secondLine || !antialiased {
			return fmt.Errorf("preview at %g×: second line=%t, antialiasing=%t", scale, secondLine, antialiased)
		}
		line, err := preview.Render(context.Background(), path, 0, nil, strings.Repeat(sample+" ", 8), 28, 1.4, 0, 150, scale, color.RGBA{R: 35, G: 39, B: 46, A: 255})
		if err != nil {
			return fmt.Errorf("single-line preview at %g×: %w", scale, err)
		}
		tail := false
		for y := 0; y < line.Rect.Dy(); y++ {
			for x := 0; x < line.Rect.Dx(); x++ {
				if line.RGBAAt(x, y).A == 0 {
					continue
				}
				if y >= int(65*scale) {
					return fmt.Errorf("single-line preview at %g× wrapped", scale)
				}
				if x >= int(260*scale) {
					tail = true
				}
			}
		}
		if !tail || line.Rect.Dy() != wrapped.Rect.Dy() {
			return fmt.Errorf("single-line preview at %g× clipped or resized", scale)
		}
	}
	for _, axis := range faces[0].Axes {
		if axis.Minimum == axis.Maximum {
			continue
		}
		other, renderErr := preview.Render(context.Background(), path, 0, map[string]float32{axis.Tag: axis.Maximum}, sample, 28, 1.4, 260, 52, 1, color.RGBA{R: 35, G: 39, B: 46, A: 255})
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
	entries, err := fonts.ReadIndex(path)
	if err != nil || len(entries) != len(faces) {
		return fmt.Errorf("font index read: %v", err)
	}
	for i, entry := range entries {
		if entry != faces[i].Entry {
			return fmt.Errorf("font index entry mismatch")
		}
		for _, name := range faces[i].Names {
			if name.ID == 1 && name.Decoded && name.Value != "" && entry.Family == "" {
				return fmt.Errorf("font index family name is empty")
			}
		}
	}
	selected, err := fonts.ReadFace(path, face.Index)
	if err != nil || selected.Entry != face.Entry || !slices.Equal(selected.Characters, face.Characters) || len(selected.Names) != len(face.Names) {
		return fmt.Errorf("font details read mismatch: %v", err)
	}
	if err := cache.SaveFaces(path, face.Size, face.Modified, entries); err != nil {
		return err
	}
	reloaded, found := cache.Faces(path, face.Size, face.Modified)
	if !found || !slices.Equal(reloaded, entries) {
		return fmt.Errorf("font index cache mismatch")
	}
	key := coverageKey(face.Entry)
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
		if saved.ThemeMode >= 0 && saved.ThemeMode <= 2 {
			a.themeMode = saved.ThemeMode
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
	data, err := json.MarshalIndent(settings{Sources: a.sources, Scope: a.scope, Open: open, Charsets: a.charsets, CharsetRevision: a.charsetRevision, ThemeMode: a.themeMode}, "", "  ")
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
	a.systemIDs = make(map[string]bool)
	a.recount()
	roots := slices.Clone(a.sources)
	go func() {
		seen := make(map[string]struct{})
		type scanFace struct {
			face   fonts.Entry
			system bool
		}
		batch := make([]scanFace, 0, 32)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			ready := append([]scanFace(nil), batch...)
			batch = batch[:0]
			a.window.Update(func() {
				if current != a.scanID {
					return
				}
				for _, item := range ready {
					face := item.face
					if item.system && !a.systemIDs[face.ID()] {
						a.systemIDs[face.ID()] = true
						if _, exists := a.faceIndex[face.ID()]; exists {
							a.addScopeCount("system", fontNameKey(face))
						}
					}
					if index, exists := a.faceIndex[face.ID()]; exists {
						a.faces[index] = face
					} else {
						a.faceIndex[face.ID()] = len(a.faces)
						a.faces = append(a.faces, face)
						a.addCount(face)
					}
				}
				a.rebuild()
			})
		}
		enqueue := func(face fonts.Entry, system bool) {
			id := face.ID()
			if _, exists := seen[id]; exists {
				return
			}
			seen[id] = struct{}{}
			batch = append(batch, scanFace{face: face, system: system})
			if len(batch) == cap(batch) {
				flush()
			}
		}
		report := func(message string) {
			a.window.Update(func() {
				if current == a.scanID {
					a.errors = append(a.errors, message)
				}
			})
		}
		wanted := make(map[string]map[int]bool)
		if err := fonts.SystemFaces(ctx, func(path string, index int) {
			if wanted[path] == nil {
				wanted[path] = make(map[int]bool)
			}
			wanted[path][index] = true
		}, func(err error) { report(err.Error()) }); err != nil && ctx.Err() == nil {
			report("DirectWrite 系统字体枚举: " + err.Error())
		}
		paths := make([]string, 0, len(wanted))
		for path := range wanted {
			paths = append(paths, path)
		}
		slices.SortFunc(paths, func(left, right string) int { return strings.Compare(strings.ToLower(left), strings.ToLower(right)) })
		for _, path := range paths {
			if ctx.Err() != nil {
				break
			}
			faces, err := a.readFaces(path)
			if err != nil {
				report(fmt.Sprintf("%s: %v", path, err))
				continue
			}
			for _, face := range faces {
				if wanted[path][face.Index] {
					enqueue(face, true)
				}
			}
		}
		flush()
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
			}, func(face fonts.Entry) {
				enqueue(face, false)
			}, func(path string, err error) {
				report(fmt.Sprintf("%s: %v", path, err))
			})
			flush()
		}
		a.window.Update(func() {
			if current == a.scanID {
				if ctx.Err() == nil {
					a.faces = slices.DeleteFunc(a.faces, func(face fonts.Entry) bool {
						_, exists := seen[face.ID()]
						return !exists
					})
					a.faceIndex = make(map[string]int, len(a.faces))
					for index, face := range a.faces {
						a.faceIndex[face.ID()] = index
					}
					a.recount()
				}
				a.scanning = false
				a.cancelScan = nil
				a.rebuild()
			}
		})
	}()
}

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

func (a *appState) detailsMatch(value string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(a.detailsQuery))
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

func (a *appState) view(c *ui.Context) {
	switch a.themeMode {
	case 1:
		c.SetTheme(ui.LightTheme())
	case 2:
		c.SetTheme(ui.DarkTheme())
	}
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
	width, _ := c.Size()
	if width >= 1024 {
		a.leftWidth = min(a.leftWidth, width-800)
		a.centerWidth = min(a.centerWidth, width-a.leftWidth-320)
	}
	ui.Split(c, &a.leftWidth, func() { a.sidebar(c) }, func() {
		ui.Split(c, &a.centerWidth, func() { a.browser(c) }, func() { a.details(c) }).Fill()
	}).Fill()
}

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

func (a *appState) compareView(c *ui.Context) {
	faces := make([]fonts.Entry, 0, len(a.compareIDs))
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
		a.previewToolbar(c)
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
							a.previewView(c, face, a.previewSize*1.5, 450, 240)
						}).Border(1, c.Theme().Border).Radius(8)
					}
				})
			}
		})
	})
}

func (a *appState) previewToolbar(c *ui.Context) {
	ui.Row(c).Gap(8).Children(func() {
		if ui.TextArea(c, &a.previewText).Placeholder("输入多行预览文案").Height(68).Grow(1).Changed() {
			a.previewPreset = "自定义"
		}
		labels := []string{"自定义"}
		for _, preset := range preview.Presets {
			labels = append(labels, preset.Label)
		}
		if ui.Select(c, &a.previewPreset, labels).Changed() {
			for _, preset := range preview.Presets {
				if preset.Label == a.previewPreset {
					a.previewText = preset.Text
					break
				}
			}
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "字号 −").Clicked() && a.previewSize > 14 {
			a.previewSize -= 2
		}
		ui.Textf(c, "%.0f px", a.previewSize)
		if ui.Button(c, "字号 +").Clicked() && a.previewSize < 48 {
			a.previewSize += 2
		}
		if ui.Button(c, "行距 −").Clicked() {
			a.previewLineHeight = max(1.0, a.previewLineHeight-0.1)
		}
		ui.Textf(c, "行距 %.1f×", a.previewLineHeight)
		if ui.Button(c, "行距 +").Clicked() {
			a.previewLineHeight = min(2.0, a.previewLineHeight+0.1)
		}
	})
}

func coverageKey(face fonts.Entry) string {
	return fmt.Sprintf("%s:%d:%d", face.ID(), face.Size, face.Modified)
}

func (a *appState) coverageCacheKey(face fonts.Entry) string {
	return fmt.Sprintf("%s:charsets:%d", coverageKey(face), a.charsetRevision)
}

func (a *appState) previewKey(face fonts.Entry, sample string, size float32, width, height int, scale float32, ink color.RGBA) string {
	key := fmt.Sprintf("%s:%g:%g:%d:%d:%g:%d:%d:%d:%s", coverageKey(face), size, a.previewLineHeight, width, height, scale, ink.R, ink.G, ink.B, sample)
	axes := a.axisValues[face.ID()]
	tags := make([]string, 0, len(axes))
	for tag := range axes {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	for _, tag := range tags {
		key += fmt.Sprintf(":%s=%.3f", tag, axes[tag])
	}
	return key
}

func (a *appState) ensurePreview(face fonts.Entry, sample string, size float32, width, height int, scale float32, ink color.RGBA) string {
	if a.previewCache == nil {
		a.previewCache = make(map[string]*ui.Bitmap)
		a.previewLoading = make(map[string]bool)
		a.previewErrors = make(map[string]string)
	}
	key := a.previewKey(face, sample, size, width, height, scale, ink)
	if a.previewCache[key] != nil || a.previewLoading[key] || a.previewErrors[key] != "" {
		return key
	}
	a.previewLoading[key] = true
	lineHeight := a.previewLineHeight
	axes := make(map[string]float32)
	for tag, value := range a.axisValues[face.ID()] {
		axes[tag] = float32(value)
	}
	go func() {
		bitmap, err := preview.Render(context.Background(), face.Path, face.Index, axes, sample, size, lineHeight, width, height, scale, ink)
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

func (a *appState) previewView(c *ui.Context, face fonts.Entry, size float32, width, height int) {
	a.previewTextView(c, face, a.previewText, size, width, height)
}

func (a *appState) previewTextView(c *ui.Context, face fonts.Entry, sample string, size float32, width, height int) ui.Element {
	textColor := c.Theme().Text
	scale := float32(hidpi.GetDpiForWindow(foundation.HWND(a.window.NativeHandle()))) / 96
	key := a.ensurePreview(face, sample, size, width, height, scale, color.RGBA{R: textColor.R, G: textColor.G, B: textColor.B, A: 255})
	slot := fmt.Sprintf("%s:%d:%d:%d:%d:%d", face.ID(), width, height, textColor.R, textColor.G, textColor.B)
	if bitmap := a.previewCache[key]; bitmap != nil {
		if a.previewLast == nil || len(a.previewLast) >= 400 {
			a.previewLast = make(map[string]*ui.Bitmap)
		}
		a.previewLast[slot] = bitmap
		displayWidth := float32(width)
		if width == 0 {
			displayWidth, _ = ui.ImageSize(bitmap)
			displayWidth /= scale
		}
		image := ui.Image(c, bitmap).Size(displayWidth, float32(height)).Fit(ui.Contain)
		if width == 0 {
			image.Shrink(0)
		}
		return image
	} else if message := a.previewErrors[key]; message != "" {
		return ui.Text(c, message).TextColor(c.Theme().TextMuted).SingleLine()
	} else if bitmap := a.previewLast[slot]; bitmap != nil {
		displayWidth := float32(width)
		if width == 0 {
			displayWidth, _ = ui.ImageSize(bitmap)
			displayWidth /= scale
		}
		box := ui.Box(c).Size(displayWidth, float32(height))
		if width == 0 {
			box.Shrink(0)
		}
		return box.Children(func() {
			ui.Image(c, bitmap).Size(displayWidth, float32(height)).Fit(ui.Contain)
			ui.Text(c, "更新中…").Absolute().Right(4).Bottom(2).FontSize(12).TextColor(c.Theme().TextMuted)
		})
	} else {
		return ui.Text(c, "预览生成中…").TextColor(c.Theme().TextMuted)
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
			ui.Text(c, "待导入 · 使用上方按钮导入有版本的 UTF-8 字表").TextColor(c.Theme().TextMuted)
			return
		}
		ui.Textf(c, "%d / %d · %.2f%% · 缺 %d", row.Covered, row.Total, row.Percent, row.Missing).TextColor(c.Theme().TextMuted)
	})
}

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

func (a *appState) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "字镜").FontSize(24).Bold()
		ui.Text(c, "字体目录").TextColor(c.Theme().TextMuted)
		for row := 0; row < a.tree.Rows(); row++ {
			if a.tree.Item(row) == a.scope {
				a.treeRow = row
				break
			}
		}
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
			a.saveSettings()
		}
		if ui.Button(c, "添加文件夹").Clicked() {
			a.chooseFolder()
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
		themes := []string{"跟随系统", "浅色", "深色"}
		if ui.Button(c, "主题："+themes[a.themeMode]).Clicked() {
			a.themeMode = (a.themeMode + 1) % len(themes)
			a.saveSettings()
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

func (a *appState) browser(c *ui.Context) {
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
		sortTitle := "Name（字体名称）"
		if a.sortMode == 0 {
			sortTitle += " · 原始顺序"
		}
		nameWidth := max(120, (a.centerWidth-32-52-220)*2/3)
		columns := []ui.TableColumn{{Title: "对比", Width: 52}, {Title: sortTitle, ID: "name", Width: nameWidth, Sortable: true}, {Title: "预览（Family Name）", ID: "preview"}}
		listTheme := *c.Theme()
		listTheme.Spacing = 3
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
