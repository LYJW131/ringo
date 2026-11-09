package app

import (
	"mygo-music/internal/applemusic"
	"mygo-music/internal/music"
)

// The app's one source is Apple Music, in Go like the rest of it: see
// package applemusic.

// signInPage is what the source asks for when the listener signs in on a
// page of Apple's (webSignIn).
type signInPage = applemusic.WebSignIn

// newSource makes the source, which starts nothing until it's first asked.
func (a *app) newSource() music.Source {
	music.ImageHeaders = applemusic.ImageHeaders
	return applemusic.New(applemusic.Options{DataDir: userDataDir("apple-music"), WebSignIn: a.webSignIn})
}
