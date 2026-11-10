// Package video plays the moving covers Apple Music gives albums, playlists
// and artists: H.264 in fragmented MP4, decoded by libavcodec compiled to
// WebAssembly (wasm/build.sh) and run through wazero, so that the same
// decoder runs on every system Ringo builds for, without cgo.
package video

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/egoist/mygo/ui"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/h264.wasm
var h264Wasm []byte

// Supported reports whether this machine can decode at all. wazero compiles
// WebAssembly to machine code on amd64 and arm64 only; elsewhere it would
// interpret it, far too slowly for video.
func Supported() bool { return runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" }

var engine struct {
	once     sync.Once
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	err      error
	cacheDir string
}

// SetCacheDir keeps wazero's machine code for the decoder in dir, so that
// starting the app again doesn't compile it again. Call it before the first
// decoder opens.
func SetCacheDir(dir string) { engine.cacheDir = dir }

// load compiles the decoder once, the first time one opens: a few hundred
// milliseconds without a cache, which nobody waits on as the still cover
// shows meanwhile.
func load() (wazero.Runtime, wazero.CompiledModule, error) {
	engine.once.Do(func() {
		if !Supported() {
			engine.err = errors.New("video: no compiler for " + runtime.GOARCH)
			return
		}
		ctx := context.Background()
		cfg := wazero.NewRuntimeConfigCompiler()
		if engine.cacheDir != "" {
			if c, err := wazero.NewCompilationCacheWithDir(engine.cacheDir); err == nil {
				cfg = cfg.WithCompilationCache(c)
			}
		}
		rt := wazero.NewRuntimeWithConfig(ctx, cfg)
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			engine.err = err
			return
		}
		compiled, err := rt.CompileModule(ctx, h264Wasm)
		if err != nil {
			engine.err = err
			return
		}
		engine.rt, engine.compiled = rt, compiled
	})
	return engine.rt, engine.compiled, engine.err
}

// A Decoder decodes an MP4's video frames one after another, as
// ui.VideoPlayer asks, playing it again from the start on Rewind. Each has
// a decoder of its own, in WebAssembly memory of its own; a player calls it
// from one goroutine at a time.
type Decoder struct {
	mod api.Module
	mem api.Memory
	fn  struct{ malloc, send, receive, flush, rgba, close api.Function }
	ctx context.Context
	dec uint64
	// buf is a place in the module's memory for a sample, of room bytes;
	// out is two int32s, for the size h264_rgba writes.
	buf, room, out uint32

	samples  []mp4.FullSample
	next     int
	draining bool
	frame    time.Duration
	img      image.RGBA
}

// Play opens data, an MP4 of H.264, for a player: with the machine's
// video decoder where there is one (Hardware), else with FFmpeg's, as also
// where the machine's won't take it.
func Play(data []byte) (ui.VideoDecoder, error) {
	if Hardware() {
		if d, err := openHardware(data); err == nil {
			return d, nil
		}
	}
	return Open(data)
}

// Open makes a decoder of a fragmented MP4 holding H.264, as Apple's
// moving covers are. data stays in use until the decoder closes.
func Open(data []byte) (*Decoder, error) {
	samples, avcC, frame, err := demux(data)
	if err != nil {
		return nil, err
	}
	rt, compiled, err := load()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, err
	}
	d := &Decoder{mod: mod, mem: mod.Memory(), ctx: ctx, samples: samples, frame: frame}
	d.fn.malloc = mod.ExportedFunction("h264_malloc")
	d.fn.send = mod.ExportedFunction("h264_send")
	d.fn.receive = mod.ExportedFunction("h264_receive")
	d.fn.flush = mod.ExportedFunction("h264_flush")
	d.fn.rgba = mod.ExportedFunction("h264_rgba")
	d.fn.close = mod.ExportedFunction("h264_close")
	fail := func(err error) (*Decoder, error) {
		mod.Close(ctx)
		return nil, err
	}
	if d.out, err = d.alloc(8); err != nil {
		return fail(err)
	}
	p, err := d.put(avcC)
	if err != nil {
		return fail(err)
	}
	r, err := mod.ExportedFunction("h264_open").Call(ctx, uint64(p), uint64(len(avcC)))
	if err != nil {
		return fail(err)
	}
	if r[0] == 0 {
		return fail(errors.New("video: the decoder didn't take the stream"))
	}
	d.dec = r[0]
	return d, nil
}

func (d *Decoder) alloc(n int) (uint32, error) {
	r, err := d.fn.malloc.Call(d.ctx, uint64(n))
	if err != nil {
		return 0, err
	}
	if r[0] == 0 {
		return 0, errors.New("video: out of memory")
	}
	return uint32(r[0]), nil
}

// put copies b into the module's memory, at buf, which grows as samples do.
func (d *Decoder) put(b []byte) (uint32, error) {
	if uint32(len(b)) > d.room {
		n := max(len(b), 64<<10)
		p, err := d.alloc(n)
		if err != nil {
			return 0, err
		}
		d.buf, d.room = p, uint32(n)
	}
	if !d.mem.Write(d.buf, b) {
		return 0, errors.New("video: a sample out of memory's bounds")
	}
	return d.buf, nil
}

// Next decodes the next frame, with io.EOF after the last. Its image is in
// the module's memory, and good until the next call, as ui.VideoPlayer
// copies it at once.
func (d *Decoder) Next() (ui.VideoFrame, error) {
	for {
		r, err := d.fn.receive.Call(d.ctx, d.dec)
		if err != nil {
			return ui.VideoFrame{}, err
		}
		switch int32(r[0]) {
		case 1:
			return d.picture()
		case 2:
			return ui.VideoFrame{}, io.EOF
		case 0:
		default:
			return ui.VideoFrame{}, fmt.Errorf("video: decoding failed (%d)", int32(r[0]))
		}
		if d.next >= len(d.samples) {
			if d.draining {
				return ui.VideoFrame{}, io.EOF
			}
			// What the decoder holds back to put B-frames in order comes
			// out once it's told there's no more.
			d.draining = true
			if _, err := d.fn.send.Call(d.ctx, d.dec, 0, 0); err != nil {
				return ui.VideoFrame{}, err
			}
			continue
		}
		s := d.samples[d.next].Data
		d.next++
		p, err := d.put(s)
		if err != nil {
			return ui.VideoFrame{}, err
		}
		r, err = d.fn.send.Call(d.ctx, d.dec, uint64(p), uint64(len(s)))
		if err != nil {
			return ui.VideoFrame{}, err
		}
		if int32(r[0]) < 0 {
			return ui.VideoFrame{}, fmt.Errorf("video: a sample didn't decode (%d)", int32(r[0]))
		}
	}
}

func (d *Decoder) picture() (ui.VideoFrame, error) {
	r, err := d.fn.rgba.Call(d.ctx, d.dec, uint64(d.out))
	if err != nil {
		return ui.VideoFrame{}, err
	}
	if r[0] == 0 {
		return ui.VideoFrame{}, errors.New("video: a picture not 8-bit 4:2:0")
	}
	w, _ := d.mem.ReadUint32Le(d.out)
	h, _ := d.mem.ReadUint32Le(d.out + 4)
	pix, ok := d.mem.Read(uint32(r[0]), w*h*4)
	if !ok {
		return ui.VideoFrame{}, errors.New("video: a picture out of memory's bounds")
	}
	d.img = image.RGBA{Pix: pix, Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}
	return ui.VideoFrame{Image: &d.img, Duration: d.frame}, nil
}

// Rewind goes back to before the first frame.
func (d *Decoder) Rewind() error {
	if _, err := d.fn.flush.Call(d.ctx, d.dec); err != nil {
		return err
	}
	d.next, d.draining = 0, false
	return nil
}

// Close frees the decoder and its memory.
func (d *Decoder) Close() error {
	if d.mod == nil {
		return nil
	}
	d.fn.close.Call(d.ctx, d.dec)
	err := d.mod.Close(d.ctx)
	d.mod = nil
	return err
}

// Size is the size of the video's frames, from its header, before any
// decodes.
func Size(data []byte) (w, h int, err error) {
	f, err := mp4.DecodeFile(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	v, err := videoTrack(f)
	if err != nil {
		return 0, 0, err
	}
	avc := v.Mdia.Minf.Stbl.Stsd.AvcX
	return int(avc.Width), int(avc.Height), nil
}

func videoTrack(f *mp4.File) (*mp4.TrakBox, error) {
	if f.Init == nil || f.Init.Moov == nil {
		return nil, errors.New("video: not a fragmented MP4")
	}
	for _, t := range f.Init.Moov.Traks {
		if stsd := t.Mdia.Minf.Stbl.Stsd; stsd != nil && stsd.AvcX != nil && stsd.AvcX.AvcC != nil {
			return t, nil
		}
	}
	return nil, errors.New("video: no H.264 track")
}

// demux is the video track's samples, in decoding order, its avcC record,
// and how long each frame shows.
func demux(data []byte) ([]mp4.FullSample, []byte, time.Duration, error) {
	f, err := mp4.DecodeFile(bytes.NewReader(data))
	if err != nil {
		return nil, nil, 0, err
	}
	trak, err := videoTrack(f)
	if err != nil {
		return nil, nil, 0, err
	}
	id := trak.Tkhd.TrackID
	var trex *mp4.TrexBox
	if mvex := f.Init.Moov.Mvex; mvex != nil {
		for _, t := range mvex.Trexs {
			if t.TrackID == id {
				trex = t
			}
		}
	}
	var samples []mp4.FullSample
	var total uint64
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			if frag.Moof == nil || frag.Moof.Traf == nil || frag.Moof.Traf.Tfhd.TrackID != id {
				continue
			}
			s, err := frag.GetFullSamples(trex)
			if err != nil {
				return nil, nil, 0, err
			}
			for _, x := range s {
				total += uint64(x.Dur)
			}
			samples = append(samples, s...)
		}
	}
	if len(samples) == 0 {
		return nil, nil, 0, errors.New("video: no frames")
	}
	var avcC bytes.Buffer
	if err := trak.Mdia.Minf.Stbl.Stsd.AvcX.AvcC.Encode(&avcC); err != nil {
		return nil, nil, 0, err
	}
	// Frames in Apple's covers are all as long; the decoder puts them in
	// showing order, so each takes the average.
	scale := uint64(trak.Mdia.Mdhd.Timescale)
	frame := time.Second / 30
	if scale > 0 && total > 0 {
		frame = time.Duration(total * uint64(time.Second) / scale / uint64(len(samples)))
	}
	return samples, avcC.Bytes()[8:], frame, nil
}
