<div align="center">

<img src="resources/icon.png" width="112" alt="Ringo">

# Ringo

**A native desktop client for Apple Music**

Written in Go, its windows drawn by [MyGo](https://github.com/egoist/mygo)'s native UI rather than a web page. For macOS, Windows and Linux.

[简体中文](README.md) · **English**

</div>

> [!IMPORTANT]
> Ringo is a personal project. It has nothing to do with Apple and isn't endorsed by it. It uses the music.apple.com web player's own API, which isn't public and may change at any time; using Apple Music this way likely goes against Apple's terms of service, and an account may be restricted for it. Use only your own account with an active subscription, at your own risk.

## What it does

- **Home, New and Radio** as Apple Music lays them out: heroes, shelves of covers, songs in columns, what's on the air, each section with its See All.
- **Search** opens on Apple Music's categories, and lists top results, artists, albums, songs, playlists, stations and videos on one page.
- **The library** by Recently Added, Artists, Albums, Songs, Made for You and All Playlists, loading on as it scrolls, with your playlists in the sidebar.
- **Albums, artists and playlists** with the editors' notes, a background in the cover's colors, badges for how an album sounds at its best, the songs most played, and what else to hear.
- **Playback** of whole songs as 256 kbps AAC, and lossless up to 24-bit/192 kHz ALAC with a wrapper.
- **Lyrics** that follow the song word by word, with their translation where there is one.
- **Play Next and Play Last, Favorite**, adding to the library and taking out of it, adding to playlists, making and editing playlists, and AutoPlay.
- **What a desktop app has**: media keys, a tray icon, a notch player on macOS, light and dark, session restore, a song cache, loudness normalization.
- English and Simplified Chinese, in Apple Music's own words.

Not there yet: playing music videos, animated covers, Replay, social profiles, concerts.

## Screenshots

| | |
| --- | --- |
| ![New](docs/screenshots/new.jpg) | ![An artist](docs/screenshots/artist.jpg) |
| ![An album](docs/screenshots/album.jpg) | ![Search](docs/screenshots/search.jpg) |

The screenshots are drawn by the tests, off screen, with blocks of color for covers and made-up data.

## Signing in

Any one of three ways:

1. **The sign-in window**: click Sign In at the foot of the sidebar and sign in on Apple's own page. The window can't use a passkey or a security key.
2. **A media-user-token**: at [music.apple.com](https://music.apple.com), signed in, open the developer tools and copy the value of the `media-user-token` cookie into Settings › Account › Media User Token. Empty it to sign out.
3. **An account a wrapper holds**: with a wrapper-manager that has your account logged in, fill in the Wrapper Address, then the account's Apple ID or a country or region code such as `jp` in **Wrapper Account**.

Signed out, you can browse the catalog, search and open pages. Playing, the library and recommendations need an account.

## Lossless

Lossless and Hi-Res Lossless play through a [wrapper-manager](https://github.com/AMDL-Web/wrapper/tree/amdl/manager):

1. Run one and log an Apple Music account in to it.
2. Fill in **Wrapper Address** in Settings › Playback, and **Wrapper Key** if the wrapper is behind a gateway that asks for one.
3. Set Streaming to Lossless (up to 24-bit/48 kHz) or Hi-Res Lossless (up to 24-bit/192 kHz) in Settings › Playback.

Without a wrapper, or where a song has no lossless audio, it plays as AAC. Anyone who can reach the wrapper's port can use the accounts it holds: don't expose one without a key to the internet.

## Building

You need [Go](https://go.dev/dl/) 1.27 or later, and [Bun](https://bun.sh) to evaluate `mygo.config.ts`. There is no cgo, so any machine builds the apps of all three platforms.

```bash
go tool mygo dev
```

```bash
go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64
```

```bash
go test ./...
```

The apps land in `build/`: a `.app` and a `.dmg` for macOS, an `.exe` and an installer for Windows, an executable, a `.deb` and a `.tar.gz` for Linux. The macOS app isn't notarized, so the first launch has to be allowed in System Settings › Privacy & Security.

## Privacy

- No analytics, no tracking. Ringo talks to Apple's servers and to the wrapper you name, and nothing else.
- The media-user-token is kept in plain text in the app's data folder on this computer and sent only to Apple. Signing out removes this copy only.
- With Use Listening History on, which it is by default, each song is reported to Apple as it starts, as Apple's own apps do, for Recently Played, Replay and recommendations. Turn it off in Settings › General.
- Audio is decrypted on this computer. Ringo plays it; it has no way to save a song as a file.

## Credits and license

[AGPL-3.0](LICENSE).

- The player and the base of the interface come from [MyGo Music](https://github.com/egoist/mygo-music) (AGPL-3.0); the UI framework is [MyGo](https://github.com/egoist/mygo).
- The Apple Music code is a port of [mygo-music-apple-music](https://github.com/LYJW131/mygo-music-apple-music); its lossless part builds on [apple-music-downloader](https://github.com/AMDL-Web/apple-music-downloader), [wrapper](https://github.com/WorldObservationLog/wrapper) and [Temari](https://github.com/WorldObservationLog/Temari).
- The brand is described in [brand/README.md](brand/README.md). Ringo's name and mark use none of Apple's trademarks; Apple Music is a trademark of Apple Inc.
