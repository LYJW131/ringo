# AGENTS.md

Ringo is a desktop client for Apple Music: one Go program, its windows drawn by [MyGo](https://github.com/egoist/mygo)'s native UI, for macOS, Windows and Linux. It began as a fork of [MyGo Music](https://github.com/egoist/mygo-music) (the `upstream` remote), whose player and views it keeps, and its Apple Music code as lyjw's plugin for that app, ported to Go.

## Where things are

- `internal/music`: what the views and the player know of music. `page.go` is how Apple Music's pages reach the views: sections with a `Style`, of `Item`s.
- `internal/applemusic`: the one source. It asks what the web player at music.apple.com asks. `amhelper` decrypts audio and serves it on 127.0.0.1 for the player; `widevine` and `fairplay` get it its keys.
- `internal/app`: the windows. `ui_sidebar.go`, `ui_shelf.go` and `ui_detail.go` are where Apple Music's layout lives.
- `internal/player`, `internal/lyrics`, `internal/i18n`: playing, lyrics, and the app's text.
- `internal/video`: the covers that move. Apple's are H.264. On the Mac, AVFoundation decodes them on the machine's video decoder, reached through purego (`hardware_darwin.go`). Elsewhere, and where that fails, `wasm/h264.wasm` is FFmpeg's decoder alone, built by `wasm/build.sh` and run by wazero: WebAssembly, not cgo, and the same on every platform.
- `internal/heaac`: High Efficiency. It is HE-AAC, which MyGo's AAC decoder doesn't decode, so `wasm/aac.wasm` is FFmpeg's AAC decoder alone, built and run as the covers' H.264 is. AAC-LC stays with MyGo's decoder.
- `icon/`, `brand/`: the icons and the mark, drawn by `go run ./icon` and `go run ./icon/tray` (`brand/README.md`).

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
gofmt -l internal icon
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

Only write a test for behaviour that can actually break. If the code under test could change without the test noticing, or the test would fail on a harmless rewording, don't write it.

**Write tests for:**

- What input does: clicks, keys, drags, gestures and scrolling, and the state or effect they lead to (seeking when the progress bar is let go, Space after opening Now Playing, back and forward, flinging the cover, favoriting a song, opening a shelf's See All).
- Logic that doesn't need a view: parsing lyrics and Apple's answers, formatting times and counts, the player's queue, seeking, fading and cache, decoding streams and images.
- A regression you just fixed. Say in the test's comment what went wrong.
- Frame budgets, in `perf_test.go`, so idle views don't start asking for frames they don't need.

**Don't write tests that:**

- Check that a list of labels shows on screen. That repeats the view code and breaks whenever the wording changes.
- Check default settings or the order of rows in a form.
- Check a title or a count that Apple's editors change, outside the samples in `internal/applemusic/testdata`.
- Test MyGo itself, for example that `AnimateTo` or a spring moves from 0 to 1.
- Depend on the wall clock. `ui.NewTester` starts its clock at the current time, so anything with a phase taken from the clock passes or fails depending on when the test runs. Also, `FramePending` ignores frames waiting on `After` or `Painter.After`.

**How:**

- Build views with `ui.NewTester` and set up the app with `newTestApp` (in `internal/app/helpers_test.go`). Find elements by their accessibility label, then `Click`, `RightClick`, `Press`/`Move`/`Release` or `Key`, and check the app's state afterwards.
- Stand in for Apple Music with `fakeSource` (in `internal/app/helpers_test.go`), setting the methods the test needs. In `internal/applemusic`, answer the source's requests from `harness_test.go`.
- Swap the clipboard and dialogs through package variables such as `copyText`, and restore the real one with `defer`.
- See what comes back from the source's goroutine with `posts` and `runPost`.
- Look at visuals instead of asserting on them. The `MM_FRAMES` tests save PNGs into that directory and skip when it isn't set. Check pixels only for coverage, such as whether a layer still covers the window.
- Start each test with a one-sentence comment saying the behaviour it checks, in the style of the code.
- Run a new UI test with `-count=5`. Fix or delete a flaky test; don't retry it.

## What enforces what

| Rule | Enforced by |
| --- | --- |
| Every text has its Chinese, with the same verbs; no Chinese outside `internal/i18n` | `internal/i18n/catalog_test.go` |
| Commits are signed | lyjw's git configuration and hooks |
