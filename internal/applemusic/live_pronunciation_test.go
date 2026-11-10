package applemusic

import (
	"os"
	"strings"
	"testing"
)

// A song in Japanese comes with how its lines are said, in Latin letters,
// word for word as the lines are timed. Signed in, reading only:
// APPLE_MUSIC_TOKEN_FILE.
func TestLiveLyricsPronunciation(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	// 勇者, by YOASOBI.
	l, err := s.Lyrics("1707581769")
	if err != nil || l == nil {
		t.Fatalf("lyrics: %v, %v", l != nil, err)
	}
	said, wordForWord := 0, 0
	for _, line := range l.Lines {
		if line.Pronunciation == "" {
			continue
		}
		said++
		if len(line.Words) > 0 && len(line.Pronounced) == len(line.Words) {
			wordForWord++
		}
	}
	t.Logf("%d lines, %d with how they are said, %d of them word for word", len(l.Lines), said, wordForWord)
	// Not every line: one in Latin letters has none.
	if said*2 < len(l.Lines) {
		t.Errorf("%d of %d lines come with how they are said", said, len(l.Lines))
	}
	if wordForWord != said {
		t.Errorf("%d of the %d are word for word", wordForWord, said)
	}
}
