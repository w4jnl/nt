package theme

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestTokensMatchDesignSystem keeps tokens equal to docs/design-system/tokens.json, the source
// of truth: same names, same values once "{ref}" values are resolved.
func TestTokensMatchDesignSystem(t *testing.T) {
	raw, err := os.ReadFile("../../docs/design-system/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Color struct {
			Tokens []struct {
				Name  string            `json:"name"`
				Value map[string]string `json:"value"`
			} `json:"tokens"`
		} `json:"color"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	vals := map[string]map[string]string{}
	for _, tk := range doc.Color.Tokens {
		vals[tk.Name] = tk.Value
	}
	var resolve func(name, theme string, depth int) string
	resolve = func(name, theme string, depth int) string {
		v := vals[name][theme]
		if strings.HasPrefix(v, "{") && depth < 4 {
			return resolve(strings.Trim(v, "{}"), theme, depth+1)
		}
		return v
	}
	for name := range vals {
		got, ok := tokens[name]
		if !ok {
			t.Errorf("token %q is in tokens.json but not in theme.go", name)
			continue
		}
		if want := resolve(name, "dark", 0); !strings.EqualFold(got.dark, want) {
			t.Errorf("%s dark: theme.go %s, tokens.json %s", name, got.dark, want)
		}
		if want := resolve(name, "light", 0); !strings.EqualFold(got.light, want) {
			t.Errorf("%s light: theme.go %s, tokens.json %s", name, got.light, want)
		}
	}
	for name := range tokens {
		if _, ok := vals[name]; !ok {
			t.Errorf("token %q is in theme.go but not in tokens.json", name)
		}
	}
}

// TestTextContrast checks every text-on-background pair nt draws: 4.5:1 or better in both
// themes (WCAG AA for body text), as the family's design system requires.
func TestTextContrast(t *testing.T) {
	texts := []string{"ink", "ink-muted", "accent", "ok", "warn", "alert"}
	grounds := []string{"ground", "surface", "surface-sunk"}
	for _, dark := range []bool{true, false} {
		for _, fg := range texts {
			for _, bg := range grounds {
				if r := contrast(Hex(fg, dark), Hex(bg, dark)); r < 4.5 {
					t.Errorf("dark=%v %s on %s: %.2f:1, want 4.5:1", dark, fg, bg, r)
				}
			}
		}
		if r := contrast(Hex("on-accent", dark), Hex("accent", dark)); r < 4.5 {
			t.Errorf("dark=%v on-accent on accent: %.2f:1", dark, r)
		}
		if r := contrast(Hex("accent", dark), Hex("accent-soft", dark)); r < 4.5 {
			t.Errorf("dark=%v accent on accent-soft: %.2f:1", dark, r)
		}
	}
}

func TestGlyphsDistinctPerDomain(t *testing.T) {
	for _, set := range [][]State{{Synced, Syncing, Unpushed, Offline, Conflict}, {InProgress, Done, Blocked, Failed, Abandoned}} {
		seen := map[string]State{}
		for _, s := range set {
			g := Glyph(s, 0)
			if o, dup := seen[g]; dup {
				t.Errorf("%s and %s share the glyph %s", s, o, g)
			}
			seen[g] = s
		}
	}
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		s := float64(v) / 255
		if s <= 0.04045 {
			c[i] = s / 12.92
		} else {
			c[i] = math.Pow((s+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}
