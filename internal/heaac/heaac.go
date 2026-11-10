// Package heaac decodes HE-AAC, AAC with spectral band replication (and
// parametric stereo), for MyGo's MP4 decoder: Apple Music's High
// Efficiency. MyGo's own AAC decoder (audio/aac) decodes AAC-LC only, and
// nothing in Go decodes SBR, so this is libavcodec's AAC decoder compiled
// to WebAssembly (wasm/build.sh) and run by wazero, as internal/video runs
// its H.264: no cgo, and the same on every platform.
//
// Imported, it takes MP4's "mp4a" tracks over from audio/aac, and hands
// those that aren't HE-AAC back to it: AAC-LC stays with MyGo's decoder.
package heaac

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"sync"

	"github.com/egoist/mygo/audio/aac"
	"github.com/egoist/mygo/audio/mp4"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/aac.wasm
var aacWasm []byte

// audio/aac registered "mp4a" as it was imported, before this runs.
func init() {
	mp4.RegisterCodec("mp4a", open)
}

func open(asc []byte) (mp4.Codec, error) {
	cfg, ok := parseASC(asc)
	if !ok || !cfg.sbr {
		return aac.NewCodec(asc)
	}
	return newCodec(asc, cfg)
}

// config is what an AudioSpecificConfig says that this needs: the rate of
// the AAC core, and with SBR, the rate it decodes to, twice the core's.
type config struct {
	coreRate, rate int
	sbr            bool
}

var rates = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

// parseASC reads an AudioSpecificConfig (ISO/IEC 14496-3, 1.6.2.1) as far
// as it says whether there's SBR: explicitly, as object type 5 or 29
// before the core's, or, as Apple's are, after an AAC-LC config, in the
// sync extension (0x2b7) that a decoder of AAC-LC alone skips.
func parseASC(asc []byte) (config, bool) {
	r := bitReader{b: asc}
	objectType := func() int {
		t := r.bits(5)
		if t == 31 {
			t = 32 + r.bits(6)
		}
		return t
	}
	rate := func() int {
		i := r.bits(4)
		if i == 15 {
			return r.bits(24)
		}
		if i < len(rates) {
			return rates[i]
		}
		return 0
	}
	var c config
	aot := objectType()
	c.coreRate = rate()
	channels := r.bits(4)
	if aot == 5 || aot == 29 {
		c.sbr = true
		c.rate = rate()
		aot = objectType()
	} else if aot == 2 && channels != 0 {
		r.bits(1) // frameLengthFlag
		if r.bits(1) == 1 {
			r.bits(14) // coreCoderDelay
		}
		r.bits(1) // extensionFlag
		if r.left() >= 16 && r.bits(11) == 0x2b7 && objectType() == 5 && r.bits(1) == 1 {
			c.sbr = true
			c.rate = rate()
		}
	}
	if r.bad || c.coreRate == 0 {
		return config{}, false
	}
	if c.sbr && c.rate == 0 {
		c.rate = 2 * c.coreRate
	}
	if !c.sbr {
		c.rate = c.coreRate
	}
	return c, true
}

type bitReader struct {
	b   []byte
	at  int // in bits
	bad bool
}

func (r *bitReader) left() int { return len(r.b)*8 - r.at }

func (r *bitReader) bits(n int) int {
	if n > r.left() {
		r.bad = true
		r.at = len(r.b) * 8
		return 0
	}
	v := 0
	for range n {
		v = v<<1 | int(r.b[r.at/8]>>(7-r.at%8)&1)
		r.at++
	}
	return v
}

var engine struct {
	once     sync.Once
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	err      error
}

// load compiles the decoder once, as the first HE-AAC song opens: under a
// tenth of a second, while the song comes in anyway.
func load() (wazero.Runtime, wazero.CompiledModule, error) {
	engine.once.Do(func() {
		ctx := context.Background()
		rt := wazero.NewRuntime(ctx)
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			engine.err = err
			return
		}
		compiled, err := rt.CompileModule(ctx, aacWasm)
		if err != nil {
			engine.err = err
			return
		}
		engine.rt, engine.compiled = rt, compiled
	})
	return engine.rt, engine.compiled, engine.err
}

// codec is a decoder of one track's packets, in WebAssembly memory of its
// own. MyGo calls it from one goroutine at a time.
type codec struct {
	mod  api.Module
	mem  api.Memory
	fn   struct{ decode, out, flush api.Function }
	ctx  context.Context
	dec  uint64
	cfg  config
	buf  uint32 // a place in the module's memory for a packet, of room bytes
	room uint32
}

func newCodec(asc []byte, cfg config) (*codec, error) {
	rt, compiled, err := load()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, err
	}
	c := &codec{mod: mod, mem: mod.Memory(), ctx: ctx, cfg: cfg}
	c.fn.decode = mod.ExportedFunction("aac_decode")
	c.fn.out = mod.ExportedFunction("aac_out")
	c.fn.flush = mod.ExportedFunction("aac_flush")
	p, err := c.put(asc)
	if err != nil {
		mod.Close(ctx)
		return nil, err
	}
	r, err := mod.ExportedFunction("aac_open").Call(ctx, uint64(p), uint64(len(asc)))
	if err != nil || r[0] == 0 {
		mod.Close(ctx)
		return nil, errors.New("heaac: the decoder didn't take the stream")
	}
	c.dec = r[0]
	// MyGo's codecs have no Close: the module goes when the decoder does.
	runtime.AddCleanup(c, func(m api.Module) { m.Close(context.Background()) }, mod)
	return c, nil
}

// put copies b into the module's packet buffer, growing it if need be.
func (c *codec) put(b []byte) (uint32, error) {
	if uint32(len(b)) > c.room {
		room := max(uint32(len(b)), 4096)
		r, err := c.mod.ExportedFunction("aac_malloc").Call(c.ctx, uint64(room))
		if err != nil || r[0] == 0 {
			return 0, errors.New("heaac: out of memory")
		}
		if c.buf != 0 {
			c.mod.ExportedFunction("aac_free").Call(c.ctx, uint64(c.buf))
		}
		c.buf, c.room = uint32(r[0]), room
	}
	if !c.mem.Write(c.buf, b) {
		return 0, errors.New("heaac: out of memory")
	}
	return c.buf, nil
}

func (c *codec) SampleRate() int { return c.cfg.rate }

// Preroll: SBR's envelopes, as well as AAC's overlap, reach a packet back.
func (c *codec) Preroll() int { return 2 }

// PacketFrames: each packet is 1024 frames of the core, which SBR doubles.
func (c *codec) PacketFrames() int { return 1024 * c.cfg.rate / c.cfg.coreRate }

func (c *codec) Reset() { c.fn.flush.Call(c.ctx, c.dec) }

func (c *codec) Decode(dst []float32, packet []byte) ([]float32, error) {
	p, err := c.put(packet)
	if err != nil {
		return dst, err
	}
	r, err := c.fn.decode.Call(c.ctx, c.dec, uint64(p), uint64(len(packet)))
	if err != nil {
		return dst, err
	}
	n := int32(r[0])
	if n < 0 {
		return dst, errors.New("heaac: a packet that doesn't decode")
	}
	if n == 0 {
		return dst, nil
	}
	o, err := c.fn.out.Call(c.ctx, c.dec)
	if err != nil {
		return dst, err
	}
	pcm, ok := c.mem.Read(uint32(o[0]), uint32(n)*8)
	if !ok {
		return dst, errors.New("heaac: the decoder's output is out of its memory")
	}
	// FFmpeg's float goes past full scale where SBR overshoots (Apple's
	// HE-AAC peaks at 1.1), and MyGo takes samples within [-1, 1].
	for i := 0; i+4 <= len(pcm); i += 4 {
		dst = append(dst, min(max(math.Float32frombits(binary.LittleEndian.Uint32(pcm[i:])), -1), 1))
	}
	return dst, nil
}
