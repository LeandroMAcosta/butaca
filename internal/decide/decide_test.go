package decide

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/parse"
)

// Fixtures are real release names captured from Prowlarr, with the parsed
// fields produced by the actual guessit library (which emits ISO 639-1 codes,
// not English names).

func rel(title string, size int64, seeders int) indexer.Release {
	return indexer.Release{Title: title, Size: size, Seeders: seeders, MagnetURL: "magnet:?xt=" + title}
}

func gb(n float64) int64 { return int64(n * float64(1<<30)) }

func baseRules() Rules {
	return Rules{
		MinSeeders:     5,
		Resolutions:    []string{"1080p"},
		MinSize:        gb(0.5),
		MaxSize:        gb(20),
		RejectPatterns: []string{"CAM", "TS", "HDCAM"},
		LanguageMode:   LangOriginal,
	}
}

// The three films that Radarr's fixed English filter rejected outright, each of
// which had to be searched and grabbed by hand.
func TestOriginalLanguageAcceptsNonEnglishFilms(t *testing.T) {
	cases := []struct {
		name    string
		item    Item
		release indexer.Release
		parsed  parse.Result
	}{
		{
			name:    "Amelie, French, matched via original title",
			item:    Item{Titles: []string{"Amélie", "Le Fabuleux Destin d'Amélie Poulain"}, Year: 2001, OriginalLanguage: "fr"},
			release: rel("Le Fabuleux Destin d'Amélie Poulain (2001) 1080p BRRip x264 -YTS", gb(2.08), 100),
			parsed: parse.Result{Title: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001,
				ScreenSize: "1080p", ReleaseGroup: "YTS"},
		},
		{
			name:    "Exit 8, Japanese audio declared",
			item:    Item{Titles: []string{"Exit 8"}, Year: 2025, OriginalLanguage: "ja"},
			release: rel("Exit 8 2025 1080p Japanese WEB-DL HEVC x265 5.1 BONE", gb(1.44), 776),
			parsed: parse.Result{Title: "Exit 8", Year: 2025, Language: "ja",
				ScreenSize: "1080p", ReleaseGroup: "BONE"},
		},
		{
			name:    "Christiane F, German, release appends the original title",
			item:    Item{Titles: []string{"Christiane F."}, Year: 1981, OriginalLanguage: "de"},
			release: rel("Christiane F. Wir Kinder vom Bahnhof Zoo (1981) 1080p BRRip 5.1 x264 -YTS", gb(2.59), 32),
			parsed: parse.Result{Title: "Christiane F Wir Kinder vom Bahnhof Zoo", Year: 1981,
				ScreenSize: "1080p", ReleaseGroup: "YTS"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate([]indexer.Release{tc.release}, []parse.Result{tc.parsed}, tc.item, baseRules())
			if !got[0].Accepted() {
				t.Fatalf("release was rejected but should have been accepted: %v", got[0].Rejects)
			}
		})
	}
}

// A dub must still be rejected in original mode.
func TestOriginalLanguageRejectsDub(t *testing.T) {
	item := Item{Titles: []string{"Exit 8"}, Year: 2025, OriginalLanguage: "ja"}
	c := Evaluate(
		[]indexer.Release{rel("Exit 8 2025 1080p ENGLISH DUB WEB-DL x265", gb(1.5), 40)},
		[]parse.Result{{Title: "Exit 8", Year: 2025, Language: "en", ScreenSize: "1080p"}},
		item, baseRules())
	if c[0].Accepted() {
		t.Fatal("English dub of a Japanese film should be rejected in original mode")
	}
}

// "English subs" describes subtitles, not audio. Reading it as the audio
// language would reject correct releases and accept wrong ones.
func TestSubtitleLanguageIsNotAudioLanguage(t *testing.T) {
	item := Item{Titles: []string{"Amélie", "Le Fabuleux Destin d'Amélie Poulain"}, Year: 2001, OriginalLanguage: "fr"}
	p := parse.Result{Title: "Amelie", Year: 2001, SubtitleLanguage: "en", ScreenSize: ""}
	c := Evaluate([]indexer.Release{rel("Amelie 2001 English subs BluRay Rip", gb(0.73), 6)}, []parse.Result{p}, item, baseRules())

	for _, r := range c[0].Rejects {
		if r == "audio is en, want the original french" {
			t.Fatal("subtitle language was misread as audio language")
		}
	}
	// It is still rejected, but on resolution -- which is the correct reason.
	if c[0].Accepted() {
		t.Fatal("a 720p-or-unknown 0.73GB rip should not pass a 1080p rule")
	}
}

// The false positive that a naive substring match would produce: Prowlarr
// answers a "Soul" search with unrelated titles that merely contain the word.
func TestUnrelatedTitleContainingTheNameIsRejected(t *testing.T) {
	item := Item{Titles: []string{"Soul"}, Year: 2020, OriginalLanguage: "en"}
	c := Evaluate(
		[]indexer.Release{rel("Made in Abyss Dawn of the Deep Soul (2020) 1080p BluRay", gb(3), 51)},
		[]parse.Result{{Title: "Made in Abyss Dawn of the Deep Soul", Year: 2020, ScreenSize: "1080p"}},
		item, baseRules())
	if c[0].Accepted() {
		t.Fatal("a different film containing the word Soul must not match")
	}
}

// "TS" as a quality tag must not match inside an unrelated word.
func TestRejectPatternMatchesWholeTokenOnly(t *testing.T) {
	item := Item{Titles: []string{"Ghosts"}, Year: 2020, OriginalLanguage: "en"}
	c := Evaluate(
		[]indexer.Release{rel("Ghosts 2020 1080p BluRay x264", gb(2), 50)},
		[]parse.Result{{Title: "Ghosts", Year: 2020, ScreenSize: "1080p"}},
		item, baseRules())
	if !c[0].Accepted() {
		t.Fatalf("GHOSTS must not trigger the TS reject pattern: %v", c[0].Rejects)
	}
}

// Pick must prefer the healthier swarm when quality is equal.
func TestPickPrefersMoreSeeders(t *testing.T) {
	item := Item{Titles: []string{"Taxi Driver"}, Year: 1976, OriginalLanguage: "en"}
	rels := []indexer.Release{
		rel("Taxi.Driver.1976.REMASTERED.1080p.BluRay.x264.MkvCage", gb(2.69), 251),
		rel("Taxi.Driver.1976.1080p.BluRay.x264.anoXmous", gb(1.52), 23),
	}
	parsed := []parse.Result{
		{Title: "Taxi Driver", Year: 1976, ScreenSize: "1080p", ReleaseGroup: "MkvCage"},
		{Title: "Taxi Driver", Year: 1976, ScreenSize: "1080p", ReleaseGroup: "anoXmous"},
	}
	best := Pick(Evaluate(rels, parsed, item, baseRules()))
	if best == nil {
		t.Fatal("nothing was picked")
	}
	if best.Parsed.ReleaseGroup != "MkvCage" {
		t.Fatalf("expected the 251-seeder release, got %q", best.Parsed.ReleaseGroup)
	}
}

// ScoreFunc expresses judgements a fixed profile cannot -- lossless audio on a
// concert film was chosen by hand this session.
func TestScoreFuncCanPreferLosslessAudio(t *testing.T) {
	item := Item{Titles: []string{"Pink Floyd: The Wall"}, Year: 1982, OriginalLanguage: "en"}
	rels := []indexer.Release{
		rel("Pink Floyd The Wall (1982) 1080p BRRip 5.1 x264 -YTS", gb(1.88), 84),
		rel("Pink.Floyd.The.Wall.1982.1080p.bdrip.x265.5.1.FLAC-FINKLEROY", gb(4.15), 75),
	}
	parsed := []parse.Result{
		{Title: "Pink Floyd The Wall", Year: 1982, ScreenSize: "1080p", ReleaseGroup: "YTS"},
		{Title: "Pink Floyd The Wall", Year: 1982, ScreenSize: "1080p", AudioCodec: "FLAC", ReleaseGroup: "FINKLEROY"},
	}

	rules := baseRules()
	if best := Pick(Evaluate(rels, parsed, item, rules)); best.Parsed.ReleaseGroup != "YTS" {
		t.Fatalf("without a ScoreFunc the better-seeded release wins, got %q", best.Parsed.ReleaseGroup)
	}

	rules.ScoreFunc = func(c Candidate) int {
		if c.Parsed.AudioCodec == "FLAC" {
			return 60
		}
		return 0
	}
	if best := Pick(Evaluate(rels, parsed, item, rules)); best.Parsed.ReleaseGroup != "FINKLEROY" {
		t.Fatalf("ScoreFunc should have promoted the FLAC release, got %q", best.Parsed.ReleaseGroup)
	}
}

func TestParseSize(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"500MB", 500 << 20}, {"4.16GB", gb(4.16)},
		{"1 GB", 1 << 30}, {"", 0},
	} {
		got, err := ParseSize(tc.in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// Regression: a release that merely names the expected language must not beat a
// far better seeded one. Real Prowlarr data for Taxi Driver initially picked a
// 9.41GB / 16-seeder release over the 2.69GB / 251-seeder remaster because the
// language bonus outweighed a 15x seeder gap.
func TestLanguageBonusDoesNotOutweighSeeders(t *testing.T) {
	item := Item{Titles: []string{"Taxi Driver"}, Year: 1976, OriginalLanguage: "en"}
	rels := []indexer.Release{
		rel("Taxi Driver - Remastered Martin Scorsese 1976 Eng Rus Multi Subs 1080p", gb(9.41), 16),
		rel("Taxi.Driver.1976.REMASTERED.1080p.BluRay.x264.MkvCage", gb(2.69), 251),
	}
	parsed := []parse.Result{
		{Title: "Taxi Driver", Year: 1976, Language: "en", ScreenSize: "1080p"},
		{Title: "Taxi Driver", Year: 1976, ScreenSize: "1080p", ReleaseGroup: "MkvCage"},
	}
	best := Pick(Evaluate(rels, parsed, item, baseRules()))
	if best == nil {
		t.Fatal("nothing was picked")
	}
	if best.Release.Seeders != 251 {
		t.Fatalf("picked the %d-seeder release; the 251-seeder one should win", best.Release.Seeders)
	}
}

// Real Prowlarr data for Christiane F. offered a 4.50GB HDTV rip and a 2.59GB
// Blu-ray rip at nearly equal seeder counts. Both are 1080p, so only a source
// preference can tell them apart.
func TestSourcePreferenceBeatsEqualResolution(t *testing.T) {
	item := Item{Titles: []string{"Christiane F."}, Year: 1981, OriginalLanguage: "de"}
	rels := []indexer.Release{
		rel("Christiane F - Wir Kinder vom Bahnhof Zoo (1981)[1080p] Eng Subs", gb(4.50), 34),
		rel("Christiane F. Wir Kinder vom Bahnhof Zoo (1981) 1080p BRRip 5.1 x264 -YTS", gb(2.59), 32),
	}
	parsed := []parse.Result{
		{Title: "Christiane F Wir Kinder vom Bahnhof Zoo", Year: 1981, ScreenSize: "1080p", Source: "HDTV"},
		{Title: "Christiane F Wir Kinder vom Bahnhof Zoo", Year: 1981, ScreenSize: "1080p", Source: "Blu-ray", ReleaseGroup: "YTS"},
	}
	rules := baseRules()
	rules.Sources = []string{"Blu-ray", "Web", "HDTV", "DVD"}

	best := Pick(Evaluate(rels, parsed, item, rules))
	if best == nil {
		t.Fatal("nothing was picked")
	}
	if best.Parsed.Source != "Blu-ray" {
		t.Fatalf("picked the %s release; Blu-ray should outrank HDTV at equal resolution", best.Parsed.Source)
	}
}
