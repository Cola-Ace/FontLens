package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"fontlens/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func Run() {
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
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(32 << 20)
	}
	if os.Getenv("MYGO_GPU") == "" {
		os.Setenv("MYGO_GPU", "0")
	}
	app := &appState{leftWidth: defaultLeftWidth, centerWidth: defaultCenterWidth, coverageSplit: 50, scope: "all", selectedRow: -1, previewText: "FontLens 123", previewPreset: "自定义", customPreviewText: "FontLens 123", previewSize: 30, previewLineHeight: 1.5}
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
		workArea := mygo.Screen.PrimaryDisplay().WorkArea
		width := min(defaultWindowWidth, workArea.Width*9/10)
		height := min(900, workArea.Height*9/10)
		app.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "FontLens", Width: width, Height: height,
			MinWidth: min(1024, width), MinHeight: min(720, height),
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
