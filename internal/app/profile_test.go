package app

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func baseRules() decide.Rules {
	return decide.Rules{
		MinSeeders:   5,
		Resolutions:  []string{"1080p"},
		Sources:      []string{"Blu-ray", "Web"},
		LanguageMode: decide.LangOriginal,
		MinSize:      1 << 20,
		MaxSize:      20 << 30,
	}
}

// A profile states only what it wants to differ on; everything else has to
// survive untouched.
func TestApplyProfileOverridesOnlyWhatItSets(t *testing.T) {
	got, err := applyProfile(baseRules(), &store.Profile{
		Name:        "Kids",
		Resolutions: []string{"720p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resolutions) != 1 || got.Resolutions[0] != "720p" {
		t.Errorf("resolutions = %v, want [720p]", got.Resolutions)
	}
	if got.MinSeeders != 5 {
		t.Errorf("min seeders = %d, want the base 5 to survive", got.MinSeeders)
	}
	if len(got.Sources) != 2 {
		t.Errorf("sources = %v, want the base list to survive", got.Sources)
	}
	if got.MaxSize != 20<<30 {
		t.Errorf("max size = %d, want the base value to survive", got.MaxSize)
	}
}

func TestApplyProfileSwitchesLanguageMode(t *testing.T) {
	got, err := applyProfile(baseRules(), &store.Profile{
		Name: "Casa", LanguageMode: "prefer", PreferLanguage: "es",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.LanguageMode != decide.LangPrefer {
		t.Errorf("language mode = %v, want prefer", got.LanguageMode)
	}
	if got.PreferLanguage != "es" {
		t.Errorf("prefer language = %q, want es", got.PreferLanguage)
	}
}

func TestApplyProfileParsesSizes(t *testing.T) {
	got, err := applyProfile(baseRules(), &store.Profile{Name: "p", MinSize: "2GB", MaxSize: "8GB"})
	if err != nil {
		t.Fatal(err)
	}
	if got.MinSize != 2<<30 || got.MaxSize != 8<<30 {
		t.Errorf("sizes = %d..%d, want 2GB..8GB", got.MinSize, got.MaxSize)
	}
}

func TestApplyProfileReportsBadValues(t *testing.T) {
	if _, err := applyProfile(baseRules(), &store.Profile{Name: "p", LanguageMode: "nonsense"}); err == nil {
		t.Error("an invalid language_mode should be reported, not ignored")
	}
	if _, err := applyProfile(baseRules(), &store.Profile{Name: "p", MaxSize: "huge"}); err == nil {
		t.Error("an unparseable size should be reported")
	}
}
