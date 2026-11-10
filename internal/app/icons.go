package app

import "github.com/egoist/mygo/ui"

// Icons drawn on a 24-point grid, in the color of the text: transport
// controls filled, the rest in a 1.75-point stroke.

func strokeIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

func fillIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">` + paths + `</svg>`))
}

var (
	iconPlay  = fillIcon(`<path d="M7 4.6v14.8c0 .8.9 1.3 1.6.9l12-7.4c.6-.4.6-1.3 0-1.7l-12-7.4C7.9 3.3 7 3.8 7 4.6z"/>`)
	iconPause = fillIcon(`<rect x="5.5" y="4" width="4.5" height="16" rx="1.4"/><rect x="14" y="4" width="4.5" height="16" rx="1.4"/>`)
	iconNext  = fillIcon(`<path d="M3 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6L4.6 4.8C3.9 4.3 3 4.8 3 5.6z"/><path d="M12 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6l-9.4-6.4c-.7-.5-1.6 0-1.6.8z" transform="translate(-1.5 0)"/>`)
	iconPrev  = fillIcon(`<g transform="matrix(-1 0 0 1 24 0)"><path d="M3 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6L4.6 4.8C3.9 4.3 3 4.8 3 5.6z"/><path d="M12 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6l-9.4-6.4c-.7-.5-1.6 0-1.6.8z" transform="translate(-1.5 0)"/></g>`)

	iconShuffle   = strokeIcon(`<path d="M2 18h1.4c1.3 0 2.5-.6 3.3-1.7l6.1-8.6c.8-1.1 2-1.7 3.3-1.7H22"/><path d="m18 2 4 4-4 4"/><path d="M2 6h1.9c1.5 0 2.9.9 3.6 2.2"/><path d="M22 18h-5.9c-1.3 0-2.6-.7-3.3-1.8l-.5-.8"/><path d="m18 14 4 4-4 4"/>`)
	iconRepeat    = strokeIcon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/>`)
	iconRepeat1   = strokeIcon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/><path d="M11 10h1v4"/>`)
	iconVolume    = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6"/><path d="M19.4 18.4a9 9 0 0 0 0-12.8"/>`)
	iconVolumeLow = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6"/>`)
	iconMute      = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><line x1="22" x2="16" y1="9" y2="15"/><line x1="16" x2="22" y1="9" y2="15"/>`)
	iconSearch    = strokeIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconFilter    = strokeIcon(`<path d="M4 7h16M7 12h10M10 17h4"/>`)
	iconHome      = strokeIcon(`<path d="M12 3 4 9v11a1 1 0 0 0 1 1h4v-6h6v6h4a1 1 0 0 0 1-1V9z"/>`)
	iconStar      = strokeIcon(`<path d="m12 3 2.7 5.8 6.3.8-4.6 4.3 1.2 6.2L12 17l-5.6 3.1 1.2-6.2L3 9.6l6.3-.8z"/>`)
	iconStarFill  = fillIcon(`<path d="m12 3 2.7 5.8 6.3.8-4.6 4.3 1.2 6.2L12 17l-5.6 3.1 1.2-6.2L3 9.6l6.3-.8z" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"/>`)
	// iconVideo marks a music video among an album's songs, as Music's
	// screen with a note does.
	iconVideo   = strokeIcon(`<rect x="2.5" y="4.5" width="19" height="13" rx="2"/><path d="M8 21h8"/><path d="m10 8.5 4.5 2.5-4.5 2.5z"/>`)
	iconQueue   = strokeIcon(`<path d="M16 5H3"/><path d="M11 12H3"/><path d="M11 19H3"/><path d="M21 16V5"/><circle cx="18" cy="16" r="3"/>`)
	iconLyrics  = strokeIcon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/><path d="M8 9h8"/><path d="M8 13h5"/>`)
	iconDown    = strokeIcon(`<path d="m6 9 6 6 6-6"/>`)
	iconBack    = strokeIcon(`<path d="m15 18-6-6 6-6"/>`)
	iconForward = strokeIcon(`<path d="m9 18 6-6-6-6"/>`)
	iconNote    = strokeIcon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	iconUser    = strokeIcon(`<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`)
)

var (
	iconRadio = strokeIcon(`<circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49"/><path d="M7.76 16.24a6 6 0 0 1 0-8.49"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/><path d="M4.93 19.07a10 10 0 0 1 0-14.14"/>`)
	iconPlus  = strokeIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	iconCheck = strokeIcon(`<path d="m5 12 4 4L19 6"/>`)
	iconInfo  = fillIcon(`<circle cx="12" cy="5.4" r="1.75"/><path d="M9.3 9.3h4.1v8.6h1.9v1.8H8.7v-1.8h1.9v-6.8H9.3z"/>`)
	iconX     = strokeIcon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconMore  = fillIcon(`<circle cx="5" cy="12" r="1.9"/><circle cx="12" cy="12" r="1.9"/><circle cx="19" cy="12" r="1.9"/>`)
)

// The sidebar's, after Music's.
var (
	iconNew        = strokeIcon(`<rect x="3.5" y="3.5" width="7" height="7" rx="1.8"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.8"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.8"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.8"/>`)
	iconClock      = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/>`)
	iconMic        = strokeIcon(`<rect x="9" y="2.5" width="6" height="11" rx="3"/><path d="M5.5 11a6.5 6.5 0 0 0 13 0"/><path d="M12 17.5V21.5"/>`)
	iconAlbumStack = strokeIcon(`<rect x="3" y="6" width="15" height="15" rx="2.5"/><path d="M7 3h11.5A2.5 2.5 0 0 1 21 5.5V17"/><circle cx="10.5" cy="13.5" r="3"/>`)
	iconForYou     = strokeIcon(`<circle cx="9" cy="8" r="4"/><path d="M2.5 20.5a6.5 6.5 0 0 1 10.5-5.1"/><path d="M18 20.5s-3.5-2.1-3.5-4.4a1.75 1.75 0 0 1 3.5-.4 1.75 1.75 0 0 1 3.5.4c0 2.3-3.5 4.4-3.5 4.4z"/>`)
	iconExternal   = strokeIcon(`<path d="M7 17 17 7"/><path d="M8 7h9v9"/>`)
	iconGrid       = strokeIcon(`<rect x="3.5" y="3.5" width="17" height="17" rx="3"/><path d="M3.5 12h17"/><path d="M12 3.5v17"/>`)
)

var (
	iconUpDown   = strokeIcon(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	iconFlask    = strokeIcon(`<path d="M9 3h6M10 3v6.5L4.6 18.2A2 2 0 0 0 6.3 21h11.4a2 2 0 0 0 1.7-2.8L14 9.5V3M7.5 15h9"/>`)
	iconSettings = strokeIcon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	iconSignOut  = strokeIcon(`<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>`)
)

// speakerBody is a speaker, filled, at the left of its icon.
const speakerBody = `<path fill="currentColor" d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/>`

// The speakers at the ends of Now Playing's volume, filled, as heavy as the
// glyphs over them: quiet, loud, and silent.
var (
	iconSpeaker     = strokeIcon(speakerBody)
	iconSpeakerLoud = strokeIcon(speakerBody + `<path stroke-width="2" d="M15.5 9.2a4.6 4.6 0 0 1 0 5.6"/><path stroke-width="2" d="M18.7 6.4a8.6 8.6 0 0 1 0 11.2"/>`)
	iconSpeakerMute = strokeIcon(speakerBody + `<path stroke-width="2" d="m21.5 9.5-5 5"/><path stroke-width="2" d="m16.5 9.5 5 5"/>`)
)

// iconInfinity marks AutoPlay, which goes on without end, as Apple Music's
// does.
var iconInfinity = strokeIcon(`<path d="M6 16c5 0 7-8 12-8a4 4 0 0 1 0 8c-5 0-7-8-12-8a4 4 0 1 0 0 8"/>`)

// iconTranslate is two scripts side by side: what the lyrics show under
// their lines, a translation or how they are said.
var iconTranslate = strokeIcon(`<path d="m5 8 6 6"/><path d="m4 14 6-6 2-3"/><path d="M2 5h12"/><path d="M7 2h1"/><path d="m22 22-5-10-5 10"/><path d="M14 18h6"/>`)
