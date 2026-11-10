package app

import (
	"strings"

	"ringo/internal/applemusic"
	"ringo/internal/music"
)

// The app's one source is Apple Music, in Go like the rest of it: see
// package applemusic.

// signInPage is what the source asks for when the listener signs in on a
// page of Apple's (webSignIn).
type signInPage = applemusic.WebSignIn

// newSource makes the source, which starts nothing until it's first asked.
func (a *app) newSource() music.Source {
	music.ImageHeaders = applemusic.ImageHeaders
	return applemusic.New(applemusic.Options{DataDir: userDataDir("apple-music"), WebSignIn: a.webSignIn, Region: systemRegion()})
}

// systemRegion is the country or region of the first of the system's
// languages that names one, in small letters, as cn of zh-Hans-CN: where the
// source browses Apple Music until the listener signs in. "" if none does.
func systemRegion() string {
	for _, tag := range systemLanguages() {
		parts := strings.Split(strings.ReplaceAll(tag, "_", "-"), "-")
		for _, p := range parts[1:] {
			if len(p) == 2 {
				return strings.ToLower(p)
			}
		}
	}
	return ""
}
