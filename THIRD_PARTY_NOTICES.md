# Third-party notices

Ringo is under the [GNU Affero General Public License v3.0](LICENSE). It is built on, and ships with, the work of
others, under the licenses below. Each is as its own project states it; where Ringo ships a built form of something,
this says where its source is and how that form is rebuilt.

## What Ringo began as

| Work | License | In Ringo |
| --- | --- | --- |
| [MyGo Music](https://github.com/egoist/mygo-music), by EGOIST | AGPL-3.0 | Ringo is a fork of it: the player, the windows and the views it keeps are MyGo Music's, and its commits are in Ringo's history. |
| [mygo-music-apple-music](https://github.com/LYJW131/mygo-music-apple-music) | AGPL-3.0 | The Apple Music source (`internal/applemusic`) is this plugin, ported to Go. |

## Built forms that ship in the app

| Work | License | In Ringo |
| --- | --- | --- |
| [FFmpeg](https://ffmpeg.org) 8.1.3, its H.264 decoder | LGPL-2.1 or later ([text](licenses/LGPL-2.1.txt)) | `internal/video/wasm/h264.wasm`: libavcodec's H.264 decoder and libavutil alone, as WebAssembly, which plays the covers that move. |
| [FFmpeg](https://ffmpeg.org) 8.1.3, its AAC decoder | LGPL-2.1 or later ([text](licenses/LGPL-2.1.txt)) | `internal/heaac/wasm/aac.wasm`: libavcodec's AAC decoder and libavutil alone, as WebAssembly, which decodes High Efficiency (HE-AAC). |

Neither is built with FFmpeg's GPL or nonfree parts. Their source is FFmpeg's release 8.1.3
(`https://ffmpeg.org/releases/ffmpeg-8.1.3.tar.xz`, SHA-256
`7138d28c96d9d3e3af4ee3d8cad72741f8ffb40da90c1112235dea3ecd3178a3`), unchanged. `internal/video/wasm/build.sh` and
`internal/heaac/wasm/build.sh` download that release and the compiler at pinned versions and rebuild each file as it
is, with the small C files beside them (`h264.c`, `aac.c`, under Ringo's own license) around the decoders. Either
file can be replaced by one built from a changed FFmpeg, and the app rebuilt with `go build`.

## Code translated or copied into Ringo

| Work | License | In Ringo |
| --- | --- | --- |
| [Temari](https://github.com/WorldObservationLog/Temari) v0.5.1 | MIT | `internal/applemusic/fairplay` is a translation of it to Go, by way of [apple-music-downloader](https://github.com/AMDL-Web/apple-music-downloader); its test vector is Temari's (`internal/applemusic/fairplay/doc.go`). |
| [apple-music-downloader](https://github.com/AMDL-Web/apple-music-downloader) | as that project states | How lossless songs are asked of a [wrapper](https://github.com/WorldObservationLog/wrapper) follows it. |
| [MyGo](https://github.com/egoist/mygo)'s package `audio` | MIT | `internal/audio` is a copy of it, changed so that its output follows the music's own sample rate. |

## Go modules built into the app

| Module | License |
| --- | --- |
| [github.com/egoist/mygo](https://github.com/egoist/mygo) | MIT |
| [github.com/Eyevinn/mp4ff](https://github.com/Eyevinn/mp4ff) | MIT |
| [github.com/ebitengine/purego](https://github.com/ebitengine/purego) | Apache-2.0 |
| [github.com/go-text/typesetting](https://github.com/go-text/typesetting) | Unlicense or BSD-3-Clause |
| [github.com/tetratelabs/wazero](https://github.com/tetratelabs/wazero) | Apache-2.0 |
| [github.com/tphakala/go-aac](https://github.com/tphakala/go-aac) | LGPL-2.1 or later ([text](licenses/LGPL-2.1.txt)) |
| [github.com/tphakala/simd](https://github.com/tphakala/simd) | MIT |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image), [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys), [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | BSD-3-Clause |

The list is what `go list -deps .` names; `go.mod` pins each module's version, and each module carries its license's
text. go-aac is linked into the app as Go source, which `go build` relinks from a changed copy
(`go mod edit -replace`).

## Names and marks

Apple Music is a trademark of Apple Inc. Dolby and Dolby Atmos are trademarks of Dolby Laboratories. Ringo is not
affiliated with either, and draws neither's marks: where an Apple mark shows, it is in a picture Apple's service sent.
