package cmd

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestReadableHelpersClearAA is the guard on the numbers quoted in
// gui_contrast.go. Those were measured once from MyGo's palettes; if MyGo
// changes them the helpers still have to hold, and this is what notices.
func TestReadableHelpersClearAA(t *testing.T) {
	themes := map[string]*ui.Theme{
		"light": ui.LightTheme(),
		"dark":  ui.DarkTheme(),
	}

	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				what string
				got  ui.Color
				bg   ui.Color
			}{
				{"secondary on Surface", readableSecondary(th), th.Surface},
				{"secondary on Background", readableSecondary(th), th.Background},
				{"warning on Background", readableWarning(th), th.Background},
			}
			for _, tc := range cases {
				if r := contrastRatio(tc.got, tc.bg); r < aaNormal {
					t.Errorf("%s: contrast %.2f < %.1f (fg %+v on bg %+v)",
						tc.what, r, aaNormal, tc.got, tc.bg)
				}
			}
		})
	}
}

// TestReadableHelpersDoNotOvercorrect checks the helpers stop as soon as the
// threshold is met. A helper that always mixed would pass the contrast test
// while quietly flattening the interface toward one shade.
func TestReadableHelpersDoNotOvercorrect(t *testing.T) {
	// The dark palette's TextMuted clears AA on both backgrounds, so there
	// the secondary helper must hand the colour back untouched.
	th := ui.DarkTheme()
	if got := readableSecondary(th); got != th.TextMuted {
		t.Errorf("readableSecondary changed an already legible colour: %+v -> %+v", th.TextMuted, got)
	}

	// The light palette's does not: 4.40:1 on Surface is just under the 4.5
	// bar, and that gap is the whole reason the helper exists. It must
	// therefore change the colour — but only just.
	light := ui.LightTheme()
	got := readableSecondary(light)
	if got == light.TextMuted {
		t.Fatal("readableSecondary left the light palette's sub-AA TextMuted alone")
	}
	shift := contrastRatio(got, light.Surface) - aaNormal
	if shift > 0.5 {
		t.Errorf("readableSecondary overshot: %.2f is more than 0.5 above the %.1f it needed",
			contrastRatio(got, light.Surface), aaNormal)
	}
}

// TestEnsureContrastFindsTheMinimumMix checks the bisection lands on the
// smallest change that passes rather than overshooting toward the far end.
func TestEnsureContrastFindsTheMinimumMix(t *testing.T) {
	th := ui.LightTheme()

	// Warning is the worst case: it needs help on white.
	got := ensureContrast(th.Warning, th.Background, th.Text, aaNormal)
	if r := contrastRatio(got, th.Background); r < aaNormal {
		t.Fatalf("result still fails: %.2f", r)
	}
	// One step below the answer must fail, or the search overshot.
	back := th.Warning.Mix(th.Text, 0.05)
	if r := contrastRatio(back, th.Background); r >= aaNormal {
		t.Errorf("a 5%% mix already reaches %.2f, so the bisection is not minimal", r)
	}
	// And the result must not have travelled all the way to Text.
	if got == th.Text {
		t.Error("result collapsed to Text; it should stay recognisably the warning hue")
	}
}

// TestContrastRatioMatchesKnownValues checks the WCAG implementation itself,
// since every other test in this file trusts it.
func TestContrastRatioMatchesKnownValues(t *testing.T) {
	cases := []struct {
		a, b ui.Color
		want float32
	}{
		// Black on white is the 21:1 maximum.
		{ui.Hex("#000000"), ui.Hex("#ffffff"), 21},
		// A colour on itself is always 1:1.
		{ui.Hex("#2563eb"), ui.Hex("#2563eb"), 1},
		// Symmetric, and the ratio is order-independent.
		{ui.Hex("#71717a"), ui.Hex("#ffffff"), contrastRatio(ui.Hex("#ffffff"), ui.Hex("#71717a"))},
	}
	for _, tc := range cases {
		if r := contrastRatio(tc.a, tc.b); !nearEnough(r, tc.want) {
			t.Errorf("contrast(%+v, %+v) = %.3f, want %.3f", tc.a, tc.b, r, tc.want)
		}
	}
}

func nearEnough(got, want float32) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < 0.01
}
