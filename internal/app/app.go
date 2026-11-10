package app

import (
	"context"

	"fontlens/internal/coverage"
	"fontlens/internal/fonts"
	"fontlens/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	defaultWindowWidth = 1440
	defaultLeftWidth   = 208 * 4.0 / 3
	defaultDetailWidth = (defaultWindowWidth - 208 - 800 - 2) * 5.0 / 6
	defaultCenterWidth = defaultWindowWidth - defaultLeftWidth - defaultDetailWidth - 2
	defaultNameWidth   = 285
)

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
	coverageCache     map[string]coverage.Result
	coverageLoading   map[string]bool
	coveragePage      bool
	coverageMode      int
	coverageQuery     string
	coverageSplit     float32
	previewText       string
	previewPreset     string
	customPreviewText string
	previewSize       float32
	previewLineHeight float32
	previewBitmaps    map[string]*ui.Bitmap
	previewLast       map[string]*ui.Bitmap
	previewLoading    map[string]*previewRequest
	previewWanted     map[string]bool
	previewSlots      map[string]bool
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
	axisValues        map[string]map[string]float64
	selectedInstance  map[string]string
	detailsQuery      string
}

func (a *appState) view(c *ui.Context) {
	a.previewWanted = make(map[string]bool)
	a.previewSlots = make(map[string]bool)
	defer a.cancelPreviews(a.previewWanted)
	switch a.themeMode {
	case 1:
		c.SetTheme(ui.LightTheme())
	case 2:
		c.SetTheme(ui.DarkTheme())
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
	leftWidth, centerWidth := a.leftWidth, a.centerWidth
	paneScale := min(float32(1), width/1024)
	leftWidth = min(leftWidth, width-800*paneScale)
	centerWidth = min(centerWidth, width-leftWidth-280*paneScale-2)
	if ui.Split(c, &leftWidth, func() { a.sidebar(c) }, func() {
		if ui.Split(c, &centerWidth, func() { a.browser(c, centerWidth) }, func() { a.details(c) }).Fill().Changed() {
			a.centerWidth = centerWidth
		}
	}).Fill().Changed() {
		a.leftWidth = leftWidth
	}
}
