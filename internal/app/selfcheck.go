package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"fontlens/internal/codepoints"
	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"fontlens/internal/preview"
	"fontlens/internal/store"
)

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
	for _, preset := range preview.Presets {
		if strings.ContainsAny(preset.Text, "\r\n") {
			return fmt.Errorf("preview preset %s contains a line break", preset.Label)
		}
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
	expectedTotals := []int{6763, 20923, 27584, 27780, 97908, 8105, 3500, 3000, 1605, 13706, 5033, 6879, 11209, 8224, 11172}
	if len(result.Charsets) != len(expectedTotals) {
		return fmt.Errorf("builtin charset count mismatch")
	}
	for i, total := range expectedTotals {
		row := result.Charsets[i]
		if row.Pending || row.Total != total || row.Covered != 0 || row.Missing != total || row.Percent != 0 {
			return fmt.Errorf("charset denominator mismatch: %s", row.Name)
		}
	}
	allCharacters := codepoints.Set{{First: 0, Last: unicode.MaxRune}}
	for _, row := range coverage.Builtin(allCharacters) {
		if row.Covered != row.Total || row.Missing != 0 || row.Percent != 100 {
			return fmt.Errorf("full charset coverage mismatch: %s", row.Name)
		}
	}
	// A supplementary-plane character must contribute to coverage as one codepoint.
	for _, row := range coverage.Builtin(codepoints.New([]rune{'\U00020000'})) {
		want := 0
		if strings.HasPrefix(row.Name, "GB18030-2022 级别 3") {
			want = 1
		}
		if row.Covered != want {
			return fmt.Errorf("supplementary charset coverage mismatch: %s", row.Name)
		}
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
	missing := ""
	for _, character := range []rune{'汉', '😀', '\U0010FFFF', '\uFFFF'} {
		if !faces[0].Characters.Contains(character) {
			missing += string(character)
		}
	}
	if missing != "" {
		blank, err := preview.Render(context.Background(), path, 0, nil, missing, 28, 1.4, 260, 52, 1, color.RGBA{A: 255})
		if err != nil {
			return fmt.Errorf("missing-glyph preview: %w", err)
		}
		for pixel := 3; pixel < len(blank.Pix); pixel += 4 {
			if blank.Pix[pixel] != 0 {
				return fmt.Errorf("missing glyphs were drawn instead of left blank")
			}
		}
		mixed, err := preview.Render(context.Background(), path, 0, nil, sample+missing, 28, 1.4, 260, 52, 1, color.RGBA{R: 35, G: 39, B: 46, A: 255})
		if err != nil {
			return fmt.Errorf("mixed missing-glyph preview: %w", err)
		}
		if !bytes.Equal(bitmap.Pix, mixed.Pix) {
			return fmt.Errorf("missing glyphs changed the supported preview text")
		}
	}
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		short, err := preview.Render(context.Background(), path, 0, nil, "A", 28, 1.4, 260, 0, scale, color.RGBA{A: 255})
		if err != nil {
			return fmt.Errorf("auto-height preview at %g×: %w", scale, err)
		}
		longText := strings.Repeat("FontLens 123 ", 8)
		tall, err := preview.Render(context.Background(), path, 0, nil, longText, 28, 1.4, 260, 0, scale, color.RGBA{A: 255})
		if err != nil {
			return fmt.Errorf("auto-height wrapped preview at %g×: %w", scale, err)
		}
		wide, err := preview.Render(context.Background(), path, 0, nil, longText, 28, 1.4, 520, 0, scale, color.RGBA{A: 255})
		if err != nil {
			return fmt.Errorf("auto-height wide preview at %g×: %w", scale, err)
		}
		if short.Rect.Dy() <= 0 || tall.Rect.Dy() <= short.Rect.Dy() || wide.Rect.Dy() >= tall.Rect.Dy() || tall.Rect.Dx() != int(math.Ceil(float64(260*scale))) {
			return fmt.Errorf("auto-height preview at %g× did not adapt to text and width", scale)
		}
		for _, centered := range []*image.RGBA{short, tall, wide} {
			top, bottom := centered.Rect.Dy(), -1
			for y := 0; y < centered.Rect.Dy(); y++ {
				for x := 0; x < centered.Rect.Dx(); x++ {
					if centered.RGBAAt(x, y).A > 0 {
						top, bottom = min(top, y), max(bottom, y)
					}
				}
			}
			if bottom >= top && math.Abs(float64(top-(centered.Rect.Dy()-1-bottom))) > 2 {
				return fmt.Errorf("auto-height preview at %g× is not vertically centered", scale)
			}
		}
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
