package app

import (
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.win == nil {
		return // no app to show a window in, as in tests
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.measureCache()
		return
	}
	a.measureCache()
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:     T("Settings"),
		Width:     780,
		Height:    640,
		MinWidth:  640,
		MinHeight: 420,
		// Its sidebar reaches up under the window buttons, as the main window's does.
		TitleBarStyle:        mygo.TitleBarHidden,
		TitleBarHeight:       buttonsHeight,
		TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
		// A new key: the size kept for the old, narrower window doesn't fit this.
		StateKey:        "settings-pages",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		// Its sign-in dialog goes with it; a sign-in on its way still counts.
		if a.signIn.here {
			a.signIn.open = false
		}
		a.syncDock()
	})
	a.syncDock()
}

// openAccountSettings shows the page of the settings where the listener signs
// in, which is where Apple Music's errors ask to be taken.
func (a *app) openAccountSettings() {
	a.settingsPage, a.settingsQuery = "account", ""
	a.openSettings()
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

// systemLanguages are the languages the system prefers; tests replace it.
var systemLanguages = func() []string { return mygo.App.PreferredLanguages() }

// setLanguage speaks another language from now on: the views say it as they
// redraw; the menus, and what the sources wrote out in it, are made anew.
func (a *app) setLanguage(code string) {
	a.settings.Language = code
	i18n.SetLanguage(i18n.Resolve(code, systemLanguages()))
	a.changed()
	if a.win != nil {
		a.setMenu()
		a.refreshTray()
	}
	if a.settingsWin != nil {
		a.settingsWin.SetTitle(T("Settings"))
	}
	// The source's pages are in the language they were asked in.
	a.libraryChanged(a.library)
}

// choice is a select of labels, to translate, reporting the index of the one
// chosen once it changes.
func choice(c *ui.Context, key, label string, labels []string, at int) (int, bool) {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = T(l)
	}
	chosen := names[max(at, 0)]
	if ui.Select(c.Key(key), &chosen, names).Label(label).Width(170).Changed() {
		for i, n := range names {
			if n == chosen {
				return i, true
			}
		}
	}
	return at, false
}

// settingsRow is a setting of the settings window: what it is, what it means,
// and its control. It shows if a search finds it.
type settingsRow func(label, desc string, control func())

// settingsBlock is anything else in the settings, which a search finds by text.
type settingsBlock func(text string, draw func())

// settingsGroup is a card of a page of the settings, under a small title if it
// has one. Its build only says what it has through row and block, for a
// search to go through them without drawing.
type settingsGroup struct {
	title string
	build func(row settingsRow, block settingsBlock)
}

// settingsPage is a page of the settings, a column of groups.
type settingsPage struct {
	id, title string
	icon      *ui.SVG
	groups    []settingsGroup
}

// settingsMatch reports whether a search, lower case, finds any of the texts.
func settingsMatch(query string, texts ...string) bool {
	for _, t := range texts {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	useAccent(&t, p)
	c.SetTheme(&t)
	c.Root().Background(p.bg)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.settingsSearchBox.Focus(c) })

	pages := a.settingsPages(c, p)
	current := pages[0]
	for _, pg := range pages {
		if pg.id == a.settingsPage {
			current = pg
		}
	}
	query := strings.ToLower(strings.TrimSpace(a.settingsQuery))

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		// Under the window buttons where they sit over the sidebar. Where nothing
		// takes the press, it moves the window, as the title bar it reaches into would.
		top := float32(14)
		if c.TitleBar().Left > 0 {
			top = topBar(c) + 4
		}
		ui.Column(c).Width(settingsSidebarWidth).Shrink(0).Padding(top, 12, 14, 12).Gap(2).Background(p.sidebar).DragWindow().Children(func() {
			a.settingsSearch(c, p)
			for _, pg := range pages {
				a.settingsNavItem(c, p, pg, query == "" && pg.id == current.id)
			}
		})
		ui.Box(c).Width(1).Shrink(0).Background(p.hairline)
		// The page's title is in the header, the strip the window buttons leave. What
		// scrolls goes under a hard edge there, showing only once there's something under it.
		pad := max(24, topBar(c)+6)
		ui.Box(c).Grow(1).MinWidth(0).Children(func() {
			ui.Scroll(c.Key(current.id+"\x00"+query)).Fill().Padding(pad, 28, 32, 28).Gap(8).Children(func() {
				mark := ui.Box(c.Key("start-mark")).Height(0).Bounds()
				if mark.W > 0 {
					a.settingsUnderTop = mark.Y < pad-0.5
				}
				if query == "" {
					a.settingsPageView(c, p, current, "")
					return
				}
				found := false
				for _, pg := range pages {
					if settingsFinds(pg, query) {
						found = true
						a.settingsPageView(c, p, pg, query)
					}
				}
				if !found {
					ui.Text(c, T("No settings found")).FontSize(13.5).TextColor(p.muted).Margin(8, 4, 0, 4)
				}
			})
			edge := ui.Box(c.Key("top-edge")).Absolute().Top(0).Left(0).Right(0).Height(topBar(c)).DragWindow().
				Material(glass.ScrollEdge{Hard: true, Background: p.bg})
			title := current.title
			if query != "" {
				title = T("Search Results")
			}
			// The + keeps clear of the window buttons where they sit at the
			// right (Linux, Windows).
			right := max(32, c.TitleBar().Right+12)
			header := ui.Row(c.Key("header")).Absolute().Top(0).Left(0).Right(0).Height(topBar(c)).Padding(0, right, 0, 32).AlignItems(ui.Center).DragWindow()
			header.Children(func() {
				ui.Text(c, title).FontSize(15).FontWeight(700).SingleLine().Shrink(1).MinWidth(0)
			})
			// The pointer over it brings the edge up too, as AppKit's does.
			edge.Opacity(edgeShown(edge, a.settingsUnderTop || header.Hovered()))
		})
	})
	a.signInDialog(c, p, true)
}

// settingsSidebarWidth is the width of the list of the settings' pages.
const settingsSidebarWidth = 200

// settingsSearch is the field that finds settings across all the pages.
func (a *app) settingsSearch(c *ui.Context, p palette) {
	field := ui.Row(c).Height(30).Radius(8).AlignItems(ui.Stretch).Margin(0, 0, 10, 0)
	if c.Focused(a.settingsSearchBox) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent)
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		// The input fills the field, under its icon, so a click anywhere on
		// the field focuses it.
		ui.TextInputBase(c.Key("search"), &a.settingsQuery).Bind(&a.settingsSearchBox).Grow(1).MinWidth(0).PaddingStart(29).PaddingEnd(9).
			Placeholder(T("Search")).FontSize(13).Label(T("Search Settings"))
		ui.Icon(c, iconSearch).Size(13, 13).TextColor(p.muted).Absolute().Left(9).Top(7.5).IgnoreInput(true)
	})
}

// settingsNavItem opens a page of the settings, leaving a search.
func (a *app) settingsNavItem(c *ui.Context, p palette, pg settingsPage, selected bool) {
	b := ui.ButtonBase(c.Key(pg.id)).Height(32).PaddingX(9).Gap(9).Radius(8).AlignItems(ui.Center).Justify(ui.Start).
		FocusRing(false).Label(pg.title)
	switch {
	case selected:
		b.Background(p.press)
	case b.Hovered():
		b.Background(p.hover)
	}
	color := p.muted
	if selected {
		color = p.accent
	}
	b.Children(func() {
		ui.Icon(c, pg.icon).Size(15, 15).TextColor(color)
		ui.Text(c, pg.title).FontSize(13).FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
	})
	if b.Clicked() {
		a.settingsPage, a.settingsQuery = pg.id, ""
	}
}

// settingsFinds reports whether a search finds anything in a page: all of it,
// if it's what the page is called.
func settingsFinds(pg settingsPage, query string) bool {
	if settingsMatch(query, pg.title) {
		return true
	}
	for _, g := range pg.groups {
		if groupFinds(g, query) {
			return true
		}
	}
	return false
}

// groupFinds reports whether a search finds anything in a group: all of it, if
// it's what the group is called.
func groupFinds(g settingsGroup, query string) bool {
	if settingsMatch(query, g.title) {
		return true
	}
	found := false
	g.build(func(label, desc string, _ func()) {
		found = found || settingsMatch(query, label, desc)
	}, func(text string, _ func()) {
		found = found || settingsMatch(query, text)
	})
	return found
}

// settingsPageView draws the groups of a page, or of what a search finds in
// it: those it finds, and the rows of them it finds. Each is under its title,
// which a search says with the page's.
func (a *app) settingsPageView(c *ui.Context, p palette, pg settingsPage, query string) {
	shown := false
	for i, g := range pg.groups {
		whole := query == "" || settingsMatch(query, pg.title, g.title)
		if !whole && !groupFinds(g, query) {
			continue
		}
		title := g.title
		if query != "" {
			title = pg.title
			if g.title != "" {
				title += " › " + g.title
			}
		}
		// What follows another group is set apart from it.
		apart := float32(0)
		if shown {
			apart = 12
		}
		shown = true
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(apart+2, 4, 0, 4)
			apart = 0
		}
		ui.Column(c.Key(pg.id+"\x00"+strconv.Itoa(i))).Margin(apart, 0, 0, 0).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(func() {
			g.build(func(label, desc string, control func()) {
				if !whole && !settingsMatch(query, label, desc) {
					return
				}
				ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, label).FontSize(13.5).FontWeight(500)
						// A second paragraph is what the setting waits on, said
						// in the accent for it not to read as more of the first.
						what, waits, _ := strings.Cut(desc, "\n")
						if what != "" {
							ui.Text(c, what).FontSize(12).TextColor(p.muted).LineHeight(1.4)
						}
						if waits != "" {
							ui.Text(c, waits).FontSize(12).TextColor(p.accent).LineHeight(1.4).Margin(2, 0, 0, 0)
						}
					})
					control()
				})
			}, func(text string, draw func()) {
				if whole || settingsMatch(query, text) {
					draw()
				}
			})
		})
	}
}

// settingsPages are the pages of the settings, in the order Apple Music has
// them, with the groups that apply here.
func (a *app) settingsPages(c *ui.Context, p palette) []settingsPage {
	s := &a.settings

	general := settingsPage{id: "general", title: T("General"), icon: iconSettings}
	general.groups = append(general.groups, settingsGroup{"", func(row settingsRow, block settingsBlock) {
		row(T("Language"), "", func() {
			follow := T("Follow System")
			names := []string{follow}
			chosen := follow
			for _, l := range i18n.Languages {
				names = append(names, l.Name)
				if l.Code == s.Language {
					chosen = l.Name
				}
			}
			if ui.Select(c.Key("language"), &chosen, names).Label(T("Language")).Width(170).Changed() {
				code := i18n.System
				for _, l := range i18n.Languages {
					if l.Name == chosen {
						code = l.Code
					}
				}
				a.setLanguage(code)
			}
		})
		row(T("Appearance"), "", func() {
			modes := []string{"system", "light", "dark"}
			i := 0
			for k, m := range modes {
				if m == s.Appearance {
					i = k
				}
			}
			if segmented(c.Key("appearance"), &i, T("System"), T("Light"), T("Dark")) {
				s.Appearance = modes[i]
				a.changed()
			}
		})
		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if runtime.GOOS == "darwin" {
			row(T("App Icon"), T("Shown in the menu bar, it opens the window again after you close it"), func() {
				labels := make([]string, len(appIcons))
				at := 0
				for i, ic := range appIcons {
					labels[i] = ic.label
					if ic.id == s.AppIcon {
						at = i
					}
				}
				if i, ok := choice(c, "app-icon", T("App Icon"), labels, at); ok {
					s.AppIcon = appIcons[i].id
					a.changed()
					a.changeAppIcon()
				}
			})
		}
	}}, settingsGroup{"", func(row settingsRow, block settingsBlock) {
		row(T("Use Listening History"), T("Songs you play here show in Recently Played and count toward Apple Music Replay and recommendations, as in Apple's own apps"), func() {
			lib := a.library
			// The source reads "off" for it: here it's a switch.
			on := !strings.EqualFold(strings.TrimSpace(lib.src.Setting(historySetting)), "off")
			if ui.Switch(c.Key("history"), &on).Label(T("Use Listening History")).Changed() {
				value := ""
				if !on {
					value = "off"
				}
				a.keepSetting(lib, historySetting, value)
			}
		})
		row(T("Animated Art"), T("Album, playlist and artist artwork that Apple Music has in motion plays on its own, without sound"), func() {
			if ui.Switch(c.Key("animated-art"), &s.AnimatedArt).Label(T("Animated Art")).Changed() {
				a.changed()
				if !s.AnimatedArt {
					a.motion.stop()
				}
			}
		})
	}})
	// Only on macOS, the one with a notch player.
	if a.notch != nil {
		general.groups = append(general.groups, settingsGroup{T("Notch Player"), func(row settingsRow, block settingsBlock) {
			row(T("Show the Notch Player"), T("A small player at the notch on top of the screen, which opens as the pointer rests on it"), func() {
				if ui.Switch(c.Key("notch"), &s.Notch).Label(T("Show the Notch Player")).Changed() {
					a.changed()
				}
			})
			row(T("Show Activity While Playing"), T("The cover and levels beside the notch, which can cover menu bar icons there"), func() {
				if ui.Switch(c.Key("activity"), &s.NotchActivity).Label(T("Show Activity While Playing")).Disabled(!s.Notch).Changed() {
					a.changed()
				}
			})
			row(T("Keep It Showing Without a Notch"), T("When off, move the pointer to the top center of the screen to bring it up, without covering the menu bar"), func() {
				if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label(T("Keep It Showing Without a Notch")).Disabled(!s.Notch).Changed() {
					a.changed()
				}
			})
		}})
	}
	if a.updates {
		general.groups = append(general.groups, settingsGroup{T("Software Update"), func(row settingsRow, block settingsBlock) {
			desc := T("Check for a new version once a day")
			if t := lastUpdateCheck(); !t.IsZero() {
				desc = Tf("Check for a new version once a day. Last checked: %s", i18n.Ago(t, time.Now()))
			}
			row(T("Check for Updates Automatically"), desc, func() {
				on := updateChecks()
				if ui.Switch(c.Key("update-checks"), &on).Label(T("Check for Updates Automatically")).Changed() {
					setUpdateChecks(on)
				}
			})
			row(T("Download and Install Automatically"), T("Download new versions in the background, to take effect the next time the app opens"), func() {
				on := updateDownloads()
				if ui.Switch(c.Key("update-downloads"), &on).Label(T("Download and Install Automatically")).Disabled(!updateChecks()).Changed() {
					setUpdateDownloads(on)
				}
			})
			row(Tf("Current Version %s", mygo.App.Version()), "", func() {
				if ui.Button(c.Key("check-updates"), T("Check for Updates")).Clicked() {
					checkForUpdates()
				}
			})
		}})
	}

	account := settingsPage{id: "account", title: T("Account"), icon: iconUser, groups: a.accountGroups(c, p)}

	playback := settingsPage{id: "playback", title: T("Playback"), icon: iconVolume}
	playback.groups = append(playback.groups, settingsGroup{T("Audio Quality"), func(row settingsRow, block settingsBlock) {
		desc := T("AAC, 256 kbps. From the next song on")
		switch s.level() {
		case "efficient":
			desc = T("HE-AAC, 64 kbps. From the next song on")
		case "lossless":
			desc = T("ALAC, up to 24-bit/48 kHz. From the next song on")
		case "hires":
			desc = T("ALAC, up to 24-bit/192 kHz. From the next song on")
		}
		noWrapper := strings.TrimSpace(a.library.src.Setting("wrapper")) == ""
		if s.level() != "high" && noWrapper {
			desc += "\n" + T("High Efficiency, Lossless and Hi-Res Lossless play through a wrapper. Until its address is filled in below, songs play in High Quality.")
		}
		row(T("Streaming"), desc, func() {
			labels := make([]string, len(qualityLevels))
			at := 0
			for i, q := range qualityLevels {
				labels[i] = q.label
				if q.level == s.level() {
					at = i
				}
			}
			if i, ok := choice(c, "quality", T("Streaming"), labels, at); ok {
				s.Quality = qualityLevels[i].level
				a.changed()
			}
		})
		atmosDesc := T("Songs with a Dolby Atmos version play as Streaming is set.")
		switch s.atmos() {
		case music.AtmosHeadphones:
			atmosDesc = T("Songs with a Dolby Atmos version play as Apple renders it for headphones: heard in space only through headphones, and not following your head.")
		case music.AtmosSpeakers:
			atmosDesc = T("Songs with a Dolby Atmos version play as Dolby mixes it down to stereo, for speakers.")
		}
		if s.atmos() != music.AtmosOff {
			atmosDesc = Tf("%s These versions are AAC, 256 kbps, so those songs no longer play lossless.", atmosDesc)
			if noWrapper {
				atmosDesc += "\n" + T("Dolby Atmos plays through a wrapper. Until its address is filled in below, songs play as Streaming is set.")
			}
		}
		row(T("Dolby Atmos"), atmosDesc, func() {
			labels := make([]string, len(atmosModes))
			at := 0
			for i, m := range atmosModes {
				labels[i] = m.label
				if m.mode == s.atmos() || (m.mode == "off" && s.atmos() == music.AtmosOff) {
					at = i
				}
			}
			if i, ok := choice(c, "atmos", T("Dolby Atmos"), labels, at); ok {
				s.Atmos = atmosModes[i].mode
				a.changed()
			}
		})
		a.textSetting(c, p, block, "wrapper", T("Wrapper Address"), T("The wrapper-manager that holds your Apple Music account, such as http://127.0.0.1:8080."), false)
		a.textSetting(c, p, block, "wrapperKey", T("Wrapper Key"), T("The wrapper's key (API_KEY): for a wrapper-manager on Cloudflare, or one that faces the internet. Empty for one on this computer or your network without a key."), true)
	}}, settingsGroup{"", func(row settingsRow, block settingsBlock) {
		row(T("Sound Check"), T("Plays every song at the same volume"), func() {
			if ui.Switch(c.Key("normalize"), &s.Normalize).Label(T("Sound Check")).Changed() {
				a.pl.SetVolume(a.volume)
				a.changed()
			}
		})
	}})

	advanced := settingsPage{id: "advanced", title: T("Advanced"), icon: iconFlask}
	advanced.groups = append(advanced.groups, settingsGroup{T("Cache"), func(row settingsRow, block settingsBlock) {
		row(T("Song Cache"), Tf("Songs you've played are kept on this computer to play again at once. %s", a.cacheUsage()), func() {
			labels := make([]string, len(cacheLimits))
			at := 2
			for i, l := range cacheLimits {
				labels[i] = l.label
				if l.mb == s.CacheMB {
					at = i
				}
			}
			if i, ok := choice(c, "cache", T("Song Cache"), labels, at); ok {
				s.CacheMB = cacheLimits[i].mb
				a.changed()
				a.applyCacheLimit()
			}
		})
		row(T("Clear the Cache"), T("Delete all the songs kept here"), func() {
			if ui.Button(c.Key("clear-cache"), T("Clear")).Disabled(a.cacheUsed <= 0).Clicked() {
				a.clearCache()
			}
		})
	}}, settingsGroup{T("Experimental Features"), func(row settingsRow, block settingsBlock) {
		row(T("Smoke on the Progress Bar"), T("The progress bar turns into a light trailing smoke, moving with the music. Uses a little more power."), func() {
			if ui.Switch(c.Key("smoke"), &s.Smoke).Label(T("Smoke on the Progress Bar")).Changed() {
				a.changed()
			}
		})
		row(T("Fog Backdrop"), T("Now Playing's backdrop turns into fog in the cover's colors, rising with the bass. Still being polished; uses a little more power."), func() {
			if ui.Switch(c.Key("fog"), &s.Fog).Label(T("Fog Backdrop")).Changed() {
				a.changed()
			}
		})
		row(T("Grow From the Cover"), T("Now Playing opens out of the cover in the player bar and shrinks back into it, rather than sliding up from the bottom"), func() {
			if ui.Switch(c.Key("grow-from-cover"), &s.GrowFromCover).Label(T("Grow From the Cover")).Changed() {
				a.changed()
			}
		})
	}})

	return []settingsPage{general, account, playback, advanced}
}
