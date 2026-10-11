package cmd

import (
	"github.com/egoist/mygo/ui"
)

// The desktop theme, derived from MyGo's palettes rather than replacing them.
//
// MyGo ships light and dark themes that already pass AA for controls, but
// their Accent is a saturated system blue. This GUI wants a single violet
// accent across both palettes so the brand reads as one system, and it wants
// the selected row to be a tinted surface rather than a full-strength block
// — a solid highlight over a four-row table is a lot of colour for a state
// the user is only glancing at.
//
// So: take the theme MyGo hands us, override the handful of roles that carry
// the brand, and leave everything else alone. Overriding Surface and Border
// wholesale would detach the app from the platform's own light/dark
// behaviour, including the follow-system preference.

// hex builds an opaque colour from a 0xRRGGBB literal.
//
// The alpha byte matters more than it looks. MyGo reads a zero alpha as "no
// colour given" and falls back to its own default — which is how a violet
// accent silently renders as system blue. Every literal here is a solid
// colour, so the alpha is always full.
func hex(s uint32) ui.Color {
	return ui.Color{R: uint8(s >> 16), G: uint8(s >> 8), B: uint8(s), A: 0xFF}
}

// The violet ramp, from the design system. Only the steps the UI actually
// uses are named; the rest stay out of the code rather than sitting there as
// unused tokens.
var (
	violet600 = hex(0x534AB7) // accent, primary actions
	violet700 = hex(0x3C3489) // accent pressed / text on tint
	violet50  = hex(0xEEEDFE) // selected row tint, light
	violet100 = hex(0xCECBF6) // border on tint

	// A neutral that is not pure grey: the surfaces in the design system
	// carry a trace of warmth so the window does not read as clinical.
	surfaceTint  = hex(0xF7F7F5)
	surfaceTint2 = hex(0xF1EFE8)

	opaqueWhite = hex(0xFFFFFF)
)

// applyDesignTheme retunes the live theme toward the design system's violet
// accent. It is called once per frame from view(), because the theme arrives
// with the context and can flip between light and dark while the window is
// open (SetDark, or the system switching at sunset). Applying it at frame
// time means the retune follows the flip instead of stranding the window in
// whichever mode it opened in.
//
// Every override is a role the design system actually specifies. Colours it
// does not mention — Danger, Success, the pressed states — stay as MyGo
// defines them, so destructive and success affordances keep the contrast
// guarantees their own controls were built for.
func applyDesignTheme(th *ui.Theme) {
	if th == nil {
		return
	}
	th.Accent = violet600
	th.AccentHover = violet600.Mix(hex(0x3C3489), 0.35)
	th.AccentPressed = violet700

	// AccentText sits on the accent, so it has to clear AA against violet,
	// not against whatever MyGo paired with its blue.
	th.AccentText = opaqueWhite

	if th.Dark {
		// On a dark background the tint has to be strong enough to read as
		// a selection, which puts it well past the light value.
		th.Selection = violet600.Mix(hex(0xEEEDFE), 0.18)
		th.Focus = violet600
		return
	}

	th.Surface = surfaceTint
	th.Border = hex(0xE2E0D8)
	// Selection is a tint, not a block. It is also what the selected row
	// paints behind its text, so anything drawn there has to survive it —
	// see readableOnSelection.
	th.Selection = violet50
	th.Focus = violet600
}
