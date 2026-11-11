# AGENTS.md

Ringo is a desktop client for Apple Music: one Go program, its windows drawn by [MyGo](https://github.com/egoist/mygo)'s native UI, for macOS, Windows and Linux. It began as a fork of [MyGo Music](https://github.com/egoist/mygo-music) (the `upstream` remote), whose player and views it keeps, and its Apple Music code as lyjw's plugin for that app, ported to Go.

## Where things are

- `internal/music`: what the views and the player know of music. `page.go` is how Apple Music's pages reach the views: sections with a `Style`, of `Item`s.
- `internal/applemusic`: the one source. It asks what the web player at music.apple.com asks. `amhelper` decrypts audio and serves it on 127.0.0.1 for the player; `widevine` and `fairplay` get it its keys.
- `internal/app`: the windows. `ui_sidebar.go`, `ui_shelf.go` and `ui_detail.go` are where Apple Music's layout lives.
- `internal/video`, `internal/heaac`: the covers that move, and High Efficiency audio. Each has an `AGENTS.md` of its own: read it before changing that directory.
- `icon/`, `brand/`: the icons and the mark, drawn by `go run ./icon` and `go run ./icon/tray` (`brand/README.md`). `brand` is a Go package too, which embeds the mark for the app to show.

## Rules

- One source, in Go. No plugin host, no JavaScript runtime, no cgo.
- The app plays; it doesn't save decrypted audio as files of the listener's.
- Text a listener reads goes through `T`, `Tf`, `N` or `TC` with the English as a string literal, and its Simplified Chinese in `internal/i18n`. Where Apple Music has a word for a thing, use Apple Music's word in both languages. Section titles that Apple's API sends are shown as sent.
- A media-user-token or a developer token is never logged, printed, committed or passed as an argument. Tests that sign in read the token from a file an environment variable names.
- Tests against a real account read only, and print nothing that says whose it is. Tests that change an account run only with `APPLE_MUSIC_WRITE_TOKEN_FILE`, and put back what they changed.
- Comments say why, in plain sentences, in the tone of the file they're in.
- Commits are in English, a sentence for a subject, the body saying what changed and why.

## Checking a change

```bash
gofmt -l internal icon brand
go vet ./...
go test ./...
```

What only Apple can answer is tested live, and skipped without its variable:

| Variable | What runs |
| --- | --- |
| `LIVE=1` | The catalog's pages, signed out, in two storefronts |
| `APPLE_MUSIC_TOKEN_FILE` | Signed in, reading only: the account, a song decrypted, lyrics, Home, the library's pages |
| `APPLE_MUSIC_WRAPPER` with it | A song played lossless through that wrapper |
| `APPLE_MUSIC_WRITE_TOKEN_FILE` | Favorites, the library and autoplay, and an album and a playlist reported as played, on an account kept for testing |
| Both token files, and `APPLE_MUSIC_WRAPPER` | Two accounts added, switched between and removed, and one the wrapper holds switched to: reading only, the second file as a second account |

To look at the app with what Apple really answers, build it and run it with `MM_SHOTS` set to a directory: it walks its pages, saves a picture of each there, plays a song without sound and quits (`internal/app/debug_shots.go`). The sidebar is transparent in those pictures, as the window's material shows through it: lay them over a dark color before judging them.

```bash
go tool mygo build -platform darwin/arm64 -skip-dmg
```

## Tests

Before writing or changing a test, read `.claude/rules/tests.md`. Claude Code loads it by itself when it reads a `*_test.go`.

## What enforces what

| Rule | Enforced by |
| --- | --- |
| Every text has its Chinese, with the same verbs; no Chinese outside `internal/i18n` | `internal/i18n/catalog_test.go` |
| Commits are signed | lyjw's git configuration and hooks |
