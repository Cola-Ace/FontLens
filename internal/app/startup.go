package app

import (
	"log"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dwm"
	"github.com/egoist/mygo"
)

func showStartupWindow(window *mygo.Window) {
	// The hidden DirectComposition frame is retained; uncloak before the first
	// Show so DWM animates a window that already has its content.
	hwnd := foundation.HWND(window.NativeHandle())
	if err := dwm.DwmSetWindowAttribute(hwnd, uint32(dwm.DWMWA_CLOAK), []byte{0, 0, 0, 0}); err != nil {
		log.Printf("解除启动窗口遮蔽失败: %v", err)
	}
	window.Show()
}
