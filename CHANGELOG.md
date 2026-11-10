# Changelog

All notable changes to Ringo are written down here. Versions follow [Semantic Versioning](https://semver.org).

## [0.1.0]

The first version: Apple Music in a desktop app of its own, drawn by MyGo's native UI, for macOS, Windows and Linux.

### Added

- Home, New and Radio as Apple Music lays them out: heroes, shelves of covers, songs in columns, bricks, what's on the air, each with its See All.
- Search that opens on Apple Music's categories, and finds top results, artists, albums, songs, playlists, stations and videos on one page.
- The library by Recently Added, headed by when things were added, Artists, Albums, Songs, Made for You and All Playlists; Albums and Songs sort by title or by when they were added, keep to favorites and can be searched, and what comes next is fetched ahead of the scrolling. Songs you uploaded to your library play.
- Pages for albums, artists and playlists with what Apple Music shows on them: editors' notes, the cover's colors, how an album sounds at its best, the songs most played, and what else to hear; an album's record label and its page, Show Complete Album under one the library has only some of, and an artist's page headed as Music on the Mac heads it.
- Animated Art: the covers Apple Music has moving play without sound at the head of album, playlist and artist pages and on Home's tall cards, decoded by the Mac's own video decoder there, and elsewhere by FFmpeg's H.264 decoder compiled to WebAssembly.
- Whole songs as 256 kbps AAC, and through a wrapper as ALAC: Lossless up to 24-bit/48 kHz, or Hi-Res Lossless up to 24-bit/192 kHz; as High Efficiency's 64 kbps HE-AAC, decoded by FFmpeg's AAC decoder compiled to WebAssembly; and, with Dolby Atmos set to Headphones or Speakers, a song's Dolby Atmos version as Apple's binaural rendering of it or Dolby's stereo downmix, with the Dolby Atmos badge as it plays.
- Sound Check, by the loudness Apple measures of each song, and of its album when the album plays in order.
- Lyrics that follow the song word by word: the line sung sharp and the others softened with distance, a wave down the lines as one gives way to the next, light sweeping through each word, a duet's singers on two sides, backing vocals smaller, dots breathing through an interlude; translations where Apple Music has them, and lyrics without timing to read.
- Play Next and Play Last, adding to the library and taking out of it, Favorite once a thing is in the library, as in Apple Music, adding to playlists, renaming and deleting your own playlists and taking songs out of them or moving them up and down, and AutoPlay once the queue runs out.
- Signing in on Apple's own page, with a media-user-token, or as an account a wrapper holds; several accounts, switched between with a click.
- Listening history reported as Apple's web player reports it, which Recently Played follows; it can be turned off.
- English and Simplified Chinese, in Apple Music's own words.

[0.1.0]: https://github.com/LYJW131/ringo/releases/tag/v0.1.0
