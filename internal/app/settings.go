package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// settings are the listener's choices, kept in settings.json in the user
// data directory.
type settings struct {
	// Language is what the app speaks: i18n.English or i18n.Chinese, or
	// i18n.System for the system's.
	Language string `json:"language"`
	// Notch shows the quick player around the notch: the listener turns
	// it on.
	Notch bool `json:"notch"`
	// NotchActivity shows the cover and the music's levels beside the
	// notch while music plays, which hides the menu bar there.
	NotchActivity bool `json:"notchActivity"`
	// NotchVirtual keeps a notch of the app's own showing at the top of a
	// display without one; else it shows only while the pointer asks.
	NotchVirtual bool `json:"notchVirtual"`
	// NotchGreeted says the notch showed itself, once, as the first song
	// played (greet).
	NotchGreeted bool `json:"notchGreeted"`
	// Quality is the level asked of sources: efficient, high, lossless or
	// hires. One that isn't a level any more reads as high (level).
	Quality string `json:"quality"`
	// Atmos is how a song with a Dolby Atmos version plays (atmosModes):
	// off, or as its stereo version for headphones or for speakers. Apart
	// from Quality, which songs without one play at.
	Atmos string `json:"atmos,omitempty"`
	// Appearance is light, dark, or system to follow the system.
	Appearance string `json:"appearance"`
	// GrowFromCover opens Now Playing out of the player bar's cover, and
	// closes it back into it: an experiment the listener turns on. Off, it
	// slides up from the bottom of the window.
	GrowFromCover bool `json:"growFromCover"`
	// Smoke makes the progress bars comets trailing smoke as music plays,
	// for a few percent of a core: the listener turns it on.
	Smoke bool `json:"smoke"`
	// LyricsStage shows lyrics on the stage, Folia's style, in Now
	// Playing, rather than scrolling.
	LyricsStage bool `json:"lyricsStage"`
	// StageStyle is how the stage sets its lyrics (stageStyles): empty for
	// Folia's classic, or the id of another of its visualizers.
	StageStyle string `json:"stageStyle,omitempty"`
	// StageSparks throws sparks off each word on the stage as it is sung,
	// whatever its style.
	StageSparks bool `json:"stageSparks,omitempty"`
	// LibraryViews are how the library's lists were last sorted and
	// filtered: each list's page, by the list's own (ui_libsort.go).
	LibraryViews map[string]string `json:"libraryViews,omitempty"`
	// Volume is where the volume was left, from 0 to 1.
	Volume float64 `json:"volume"`
	// Normalize evens out the loudness of tracks, by what their source
	// measured of each.
	Normalize bool `json:"normalize"`
	// Autoplay goes on with music like what played once the queue ends, as
	// Apple Music's AutoPlay.
	Autoplay bool `json:"autoplay"`
	// Fog fills Now Playing's backdrop with fog of the cover's color,
	// rising and swelling with the bass: an experiment the listener turns
	// on.
	Fog bool `json:"fog"`
	// AppIcon is where the app's icon shows on macOS (appIcons); elsewhere
	// it is always in the notification area.
	AppIcon string `json:"appIcon"`
	// AnimatedArt plays the covers Apple Music has moving, where they show
	// (motion.go).
	AnimatedArt bool `json:"animatedArt"`
	// CacheMB is how many megabytes the songs kept on disk may take; 0
	// keeps none.
	CacheMB int `json:"cacheMB"`
	// SearchHistory keeps submitted keywords, most recent first.
	SearchHistory []string `json:"searchHistory,omitempty"`

	path string // where they were loaded from, and are saved to
}

func defaultSettings() settings {
	return settings{Language: i18n.System, NotchActivity: true, Quality: "hires", Appearance: "system", Volume: 0.8, Normalize: true, Autoplay: true, AnimatedArt: true, AppIcon: "dock-and-menubar", CacheMB: 2048}
}

func settingsPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// loadSettings reads the settings, or returns the defaults.
func loadSettings() settings {
	s := defaultSettings()
	if path, err := settingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &s)
		}
		s.path = path
	}
	return s
}

// save keeps the settings where they were loaded from; defaults that were
// never loaded, as in tests, are not saved.
func (s settings) save() error {
	path := s.path
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// applyAppearance makes the windows light, dark, or follow the system. Only
// a new one: it repaints every window, waiting for the main thread, and any
// setting changed applies it.
func (s settings) applyAppearance() {
	src := mygo.ThemeSystem
	switch s.Appearance {
	case "light":
		src = mygo.ThemeLight
	case "dark":
		src = mygo.ThemeDark
	}
	if src != mygo.Theme.Source() {
		mygo.Theme.SetSource(src)
	}
}

// qualityLevels are what can be asked of the source, by Apple Music's names.
var qualityLevels = []struct{ level, label string }{
	{"efficient", N("High Efficiency")},
	{"high", N("High Quality")},
	{"lossless", N("Lossless")},
	{"hires", N("Hi-Res Lossless")},
}

// level is the level asked of sources: one the settings keep that isn't a
// level any more asks for high.
func (s settings) level() string {
	for _, q := range qualityLevels {
		if q.level == s.Quality {
			return q.level
		}
	}
	return "high"
}

// atmosModes are how a song with a Dolby Atmos version can play: the
// settings keep "off" for music.AtmosOff.
var atmosModes = []struct{ mode, label string }{
	{"off", N("Off")},
	{music.AtmosHeadphones, N("Headphones")},
	{music.AtmosSpeakers, N("Speakers")},
}

// atmos is how a song with a Dolby Atmos version plays, music.AtmosOff for
// anything the settings keep that isn't a mode.
func (s settings) atmos() string {
	if s.Atmos == music.AtmosHeadphones || s.Atmos == music.AtmosSpeakers {
		return s.Atmos
	}
	return music.AtmosOff
}

// quality is what's asked of sources (music.Ask): the level, and how songs
// with a Dolby Atmos version play.
func (s settings) quality() string { return music.Ask(s.level(), s.atmos()) }

// cacheLimits are the sizes the song cache may take, in megabytes.
var cacheLimits = []struct {
	mb    int
	label string
}{
	{0, N("Off")},
	{1024, N("1 GB")},
	{2048, N("2 GB")},
	{5120, N("5 GB")},
	{10240, N("10 GB")},
}
