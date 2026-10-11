package cmd

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Readability helpers for the desktop GUI.
//
// MyGo's palette works for controls, but a few of its text pairs sit below
// the WCAG AA threshold for body text once this GUI puts secondary text on a
// Surface background or on a selected row. Measured, not guessed — these
// numbers come from the light and dark palettes in ui/theme.go:
//
//	TextMuted on Background   light 4.83   dark 6.91
//	TextMuted on Surface      light 4.40   dark 5.81   <- light fails AA (4.5)
//	TextMuted on Selection    light 3.37   dark 3.88   <- both fail AA
//	Warning   on Background   light 3.19   dark 8.25   <- light is amber on white
//
// So the three of them get a colour that clears aaNormal. Everything else
// keeps the theme's own colour: a helper that second-guessed the palette
// everywhere would drift away from it as MyGo updates.

// aaNormal is the WCAG AA contrast ratio for text below 18.66px bold or 24px
// regular. All of this GUI's body text is smaller than that, so 4.5 is the
// bar that applies.
const aaNormal = 4.5

// readableSecondary is TextMuted adjusted to stay legible on a Surface
// background. The dark palette already clears the threshold there, so it
// comes back untouched.
func readableSecondary(th *ui.Theme) ui.Color {
	return ensureContrast(th.TextMuted, th.Surface, th.Text, aaNormal)
}

// readableOnSelection is TextMuted made legible against a selected row. Both
// themes need it: a translucent accent behind muted grey lands near 3.4:1.
func readableOnSelection(th *ui.Theme) ui.Color {
	return ensureContrast(th.TextMuted, th.Selection.Over(th.Background), th.Text, aaNormal)
}

// readableWarning is the theme's Warning. On the light palette it is an
// amber that reaches only 3.19:1 on white — fine for a large glyph, not for
// a table cell — so it is pulled toward Text until it passes.
func readableWarning(th *ui.Theme) ui.Color {
	return readableWarningOn(th, th.Background)
}

// readableWarningOn is readableWarning against the surface the text is
// actually drawn on. A row's background is not always the window's: the
// chosen artifact row sits on the selection tint, and an amber tuned for
// white is close to invisible there. Passing the real background keeps one
// call from being right in one row and unreadable in the next.
func readableWarningOn(th *ui.Theme, bg ui.Color) ui.Color {
	return ensureContrast(th.Warning, bg, th.Text, aaNormal)
}

// ensureContrast moves fg toward fgAlt by the smallest amount that reaches
// minRatio against bg, and returns fg unchanged when it is already legible.
//
// It bisects on the blend factor rather than solving for it in closed form:
// ui.Color.Mix is a per-channel lerp in sRGB, and inverting the WCAG
// luminance formula through that would mean reimplementing the mix here and
// hoping the two agree. Bisecting calls the same Mix the renderer does, so it
// cannot disagree with it. Twenty steps puts the factor well inside a
// quantisation step of a uint8 channel.
func ensureContrast(fg, bg, fgAlt ui.Color, minRatio float32) ui.Color {
	if contrastRatio(fg, bg) >= minRatio {
		return fg
	}

	lo, hi := float32(0), float32(1)
	for range 20 {
		mid := (lo + hi) / 2
		if contrastRatio(fg.Mix(fgAlt, mid), bg) >= minRatio {
			hi = mid
		} else {
			lo = mid
		}
	}
	return fg.Mix(fgAlt, hi)
}

// contrastRatio is the WCAG 2.x relative-luminance ratio between two colours,
// in the range 1..21.
func contrastRatio(a, b ui.Color) float32 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance implements the WCAG formula on sRGB values. Alpha is
// ignored, so composite a translucent colour first (see readableOnSelection).
func relativeLuminance(c ui.Color) float32 {
	return 0.2126*linearize(c.R) + 0.7152*linearize(c.G) + 0.0722*linearize(c.B)
}

// linearize converts one 8-bit sRGB channel to linear light.
func linearize(v uint8) float32 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return float32(c / 12.92)
	}
	return float32(math.Pow((c+0.055)/1.055, 2.4))
}
