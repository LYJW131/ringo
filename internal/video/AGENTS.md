# internal/video

- `internal/video`: the covers that move. Apple's are H.264. On the Mac, AVFoundation decodes them on the machine's video decoder, reached through purego (`hardware_darwin.go`). Elsewhere, and where that fails, `wasm/h264.wasm` is FFmpeg's decoder alone, built by `wasm/build.sh` and run by wazero: WebAssembly, not cgo, and the same on every platform.
