package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func animateTheme(c *ui.Context) {
	theme := *c.Theme()
	root := c.Root()
	animation := ui.Animation{Duration: 250 * time.Millisecond, Ease: ui.EaseInOut, Oklab: true}
	for _, token := range []struct {
		key   string
		color *ui.Color
	}{
		{"theme-background", &theme.Background},
		{"theme-surface", &theme.Surface},
		{"theme-surface-hover", &theme.SurfaceHover},
		{"theme-surface-pressed", &theme.SurfacePressed},
		{"theme-border", &theme.Border},
		{"theme-text", &theme.Text},
		{"theme-text-muted", &theme.TextMuted},
		{"theme-accent", &theme.Accent},
		{"theme-accent-hover", &theme.AccentHover},
		{"theme-accent-pressed", &theme.AccentPressed},
		{"theme-accent-text", &theme.AccentText},
		{"theme-inactive-accent", &theme.InactiveAccent},
		{"theme-danger", &theme.Danger},
		{"theme-warning", &theme.Warning},
		{"theme-success", &theme.Success},
		{"theme-selection", &theme.Selection},
		{"theme-focus", &theme.Focus},
		{"theme-inverse", &theme.Inverse},
		{"theme-inverse-text", &theme.InverseText},
		{"theme-scrollbar", &theme.Scrollbar},
	} {
		*token.color = root.AnimateColor(token.key, *token.color, animation)
	}
	c.SetTheme(&theme)
}
