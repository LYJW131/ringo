package applemusic

import (
	"errors"
	"regexp"
	"strings"

	"mygo-music/internal/music"
)

// What the app does with an error, by its kind: signIn asks the listener to
// sign in, setup sends them to the settings, unavailable and unsupported say
// so. Without a kind it's shown as it is.

func fail(code, message string) error {
	kind := music.ErrUnsupported
	switch code {
	case "signIn":
		kind = music.ErrSignIn
	case "setup":
		kind = music.ErrSetup
	case "unavailable":
		kind = music.ErrUnavailable
	}
	return music.Fail(kind, message)
}

// coded reports whether err is one the app acts on by its kind.
func coded(err error) bool {
	return errors.Is(err, music.ErrSignIn) || errors.Is(err, music.ErrSetup) ||
		errors.Is(err, music.ErrUnavailable) || errors.Is(err, music.ErrUnsupported)
}

var catalogID = regexp.MustCompile(`^\d+$`)

func isCatalogID(id string) bool { return catalogID.MatchString(id) }

// isLibrarySong says id is a song of the listener's library, by its own ID
// rather than the catalog's: as an upload is known.
func isLibrarySong(id string) bool { return strings.HasPrefix(id, "i.") && len(id) > 2 }
