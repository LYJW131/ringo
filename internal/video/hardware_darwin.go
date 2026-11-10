//go:build darwin

package video

import (
	"errors"
	"image"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
)

// On the Mac a cover is decoded by AVFoundation, which hands it to the
// machine's video decoder: almost no CPU, where FFmpeg's in WebAssembly
// takes half a core for an artist's head at 1920×1080 (.work/apple-app/
// findings.md). It is reached through purego, not cgo, as the notch is.
// AVFoundation reads a file, so the stream is written to one for as long as
// it plays. Anything that fails leaves the cover to the WebAssembly decoder.

// coreMedia holds the C functions of CoreMedia and CoreVideo the decoder
// calls; nil where the frameworks didn't load.
type coreMedia struct {
	imageBuffer func(sample uintptr) uintptr
	lock        func(buf uintptr, flags uint64) int32
	unlock      func(buf uintptr, flags uint64) int32
	base        func(buf uintptr) unsafe.Pointer
	rowBytes    func(buf uintptr) uintptr
	width       func(buf uintptr) uintptr
	height      func(buf uintptr) uintptr
	release     func(ref uintptr)
}

var media = sync.OnceValue(func() *coreMedia {
	open := func(path string) uintptr {
		h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return 0
		}
		return h
	}
	// AVFoundation for its classes, which objc finds once it's loaded.
	if open("/System/Library/Frameworks/AVFoundation.framework/AVFoundation") == 0 {
		return nil
	}
	cm := open("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	cv := open("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	if cm == 0 || cv == 0 || cf == 0 {
		return nil
	}
	m := &coreMedia{}
	purego.RegisterLibFunc(&m.imageBuffer, cm, "CMSampleBufferGetImageBuffer")
	purego.RegisterLibFunc(&m.lock, cv, "CVPixelBufferLockBaseAddress")
	purego.RegisterLibFunc(&m.unlock, cv, "CVPixelBufferUnlockBaseAddress")
	purego.RegisterLibFunc(&m.base, cv, "CVPixelBufferGetBaseAddress")
	purego.RegisterLibFunc(&m.rowBytes, cv, "CVPixelBufferGetBytesPerRow")
	purego.RegisterLibFunc(&m.width, cv, "CVPixelBufferGetWidth")
	purego.RegisterLibFunc(&m.height, cv, "CVPixelBufferGetHeight")
	purego.RegisterLibFunc(&m.release, cf, "CFRelease")
	return m
})

// Hardware reports whether covers are decoded by the machine's video
// decoder, which takes almost no CPU at any size.
func Hardware() bool { return media() != nil }

var (
	selAlloc   = objc.RegisterName("alloc")
	selInit    = objc.RegisterName("init")
	selRelease = objc.RegisterName("release")
	selRetain  = objc.RegisterName("retain")
	selDrain   = objc.RegisterName("drain")
)

// nsString is s as an autoreleased NSString.
func nsString(s string) objc.ID {
	b := append([]byte(s), 0)
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), &b[0])
}

// pooled runs fn on one OS thread in an autorelease pool of its own: what
// AVFoundation hands back autoreleased goes when fn returns.
func pooled(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(selAlloc).Send(selInit)
	defer pool.Send(selDrain)
	fn()
}

// The pixels AVFoundation is asked for: 'BGRA', as it gives no 'RGBA'
// ("Cannot Decode"), and the player takes RGBA, so red and blue swap
// places as a frame is copied. The keys are the strings Apple's constants hold,
// kCVPixelBufferPixelFormatTypeKey and AVMediaTypeVideo, as reading a
// constant's value would take converting a symbol's address to a pointer.
const (
	pixelBGRA    = 0x42475241
	pixelKey     = "PixelFormatType"
	mediaVideo   = "vide"
	readerDone   = 2 // AVAssetReaderStatusCompleted
	lockReadOnly = 1 // kCVPixelBufferLock_ReadOnly
)

// hardware decodes a stream with AVFoundation.
type hardware struct {
	m        *coreMedia
	path     string
	asset    objc.ID
	track    objc.ID
	settings objc.ID
	reader   objc.ID
	output   objc.ID
	frame    time.Duration
	img      image.RGBA
}

// openHardware opens data, an MP4 of H.264, for AVFoundation to decode.
func openHardware(data []byte) (ui.VideoDecoder, error) {
	m := media()
	if m == nil {
		return nil, errors.New("video: AVFoundation didn't load")
	}
	f, err := os.CreateTemp("", "ringo-motion-*.mp4")
	if err != nil {
		return nil, err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	h := &hardware{m: m, path: f.Name()}
	pooled(func() { err = h.open() })
	if err != nil {
		h.Close()
		return nil, err
	}
	return h, nil
}

func (h *hardware) open() error {
	url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), nsString(h.path))
	h.asset = objc.ID(objc.GetClass("AVURLAsset")).Send(objc.RegisterName("URLAssetWithURL:options:"), url, objc.ID(0)).Send(selRetain)
	tracks := h.asset.Send(objc.RegisterName("tracksWithMediaType:"), nsString(mediaVideo))
	if tracks == 0 || objc.Send[uint](tracks, objc.RegisterName("count")) == 0 {
		return errors.New("video: no video track")
	}
	h.track = tracks.Send(objc.RegisterName("objectAtIndex:"), uint(0)).Send(selRetain)
	if rate := objc.Send[float32](h.track, objc.RegisterName("nominalFrameRate")); rate > 0 {
		h.frame = time.Duration(float64(time.Second) / float64(rate))
	}
	format := objc.ID(objc.GetClass("NSNumber")).Send(objc.RegisterName("numberWithUnsignedInt:"), uint32(pixelBGRA))
	h.settings = objc.ID(objc.GetClass("NSDictionary")).Send(objc.RegisterName("dictionaryWithObject:forKey:"), format, nsString(pixelKey)).Send(selRetain)
	return h.start()
}

// start reads the stream from its start, with a reader of its own, as one
// that has read can't go back.
func (h *hardware) start() error {
	h.stop()
	h.reader = objc.ID(objc.GetClass("AVAssetReader")).Send(selAlloc).Send(objc.RegisterName("initWithAsset:error:"), h.asset, objc.ID(0))
	if h.reader == 0 {
		return errors.New("video: AVFoundation can't read the stream")
	}
	h.output = objc.ID(objc.GetClass("AVAssetReaderTrackOutput")).Send(selAlloc).
		Send(objc.RegisterName("initWithTrack:outputSettings:"), h.track, h.settings)
	if h.output == 0 {
		return errors.New("video: AVFoundation can't decode the stream")
	}
	// The picture is copied by the player at once: no copy of the sample.
	h.output.Send(objc.RegisterName("setAlwaysCopiesSampleData:"), false)
	h.reader.Send(objc.RegisterName("addOutput:"), h.output)
	if !objc.Send[bool](h.reader, objc.RegisterName("startReading")) {
		return errors.New("video: AVFoundation didn't start reading")
	}
	return nil
}

// stop lets go of the reader.
func (h *hardware) stop() {
	if h.reader != 0 {
		h.reader.Send(objc.RegisterName("cancelReading"))
		h.reader.Send(selRelease)
		h.reader = 0
	}
	if h.output != 0 {
		h.output.Send(selRelease)
		h.output = 0
	}
}

// Next decodes the next frame, with io.EOF after the last. Its image is the
// decoder's own, drawn over by the next call, as ui.VideoPlayer copies it
// at once.
func (h *hardware) Next() (f ui.VideoFrame, err error) {
	pooled(func() { f, err = h.next() })
	return f, err
}

func (h *hardware) next() (ui.VideoFrame, error) {
	for {
		sample := uintptr(h.output.Send(objc.RegisterName("copyNextSampleBuffer")))
		if sample == 0 {
			if objc.Send[int](h.reader, objc.RegisterName("status")) == readerDone {
				return ui.VideoFrame{}, io.EOF
			}
			return ui.VideoFrame{}, errors.New("video: AVFoundation failed to decode: " + h.readerError())
		}
		buf := h.m.imageBuffer(sample)
		if buf == 0 {
			// A sample without a picture, as a stream's last may be.
			h.m.release(sample)
			continue
		}
		err := h.copyOut(buf)
		h.m.release(sample)
		if err != nil {
			return ui.VideoFrame{}, err
		}
		return ui.VideoFrame{Image: &h.img, Duration: h.frame}, nil
	}
}

// copyOut copies the picture of buf into img, red and blue swapped.
func (h *hardware) copyOut(buf uintptr) error {
	if h.m.lock(buf, lockReadOnly) != 0 {
		return errors.New("video: a picture couldn't be read")
	}
	defer h.m.unlock(buf, lockReadOnly)
	w, ht, row := int(h.m.width(buf)), int(h.m.height(buf)), int(h.m.rowBytes(buf))
	if h.img.Rect.Dx() != w || h.img.Rect.Dy() != ht {
		h.img = *image.NewRGBA(image.Rect(0, 0, w, ht))
	}
	base := h.m.base(buf)
	for y := range ht {
		src := unsafe.Slice((*uint32)(unsafe.Add(base, y*row)), w)
		dst := unsafe.Slice((*uint32)(unsafe.Pointer(&h.img.Pix[y*h.img.Stride])), w)
		for x, v := range src {
			// B G R A in memory, a little-endian word: R and B change places.
			dst[x] = v&0xff00ff00 | v>>16&0xff | v&0xff<<16
		}
	}
	return nil
}

// readerError is what the reader says went wrong.
func (h *hardware) readerError() string {
	e := h.reader.Send(objc.RegisterName("error"))
	if e == 0 {
		return "no error given"
	}
	return goString(e.Send(objc.RegisterName("description")))
}

// goString is an NSString's text.
func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	p := objc.Send[*byte](s, objc.RegisterName("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// Rewind goes back to before the first frame.
func (h *hardware) Rewind() (err error) {
	pooled(func() { err = h.start() })
	return err
}

// Close lets go of AVFoundation's reader and the stream's file.
func (h *hardware) Close() error {
	pooled(func() {
		h.stop()
		for _, id := range []*objc.ID{&h.settings, &h.track, &h.asset} {
			if *id != 0 {
				id.Send(selRelease)
				*id = 0
			}
		}
	})
	return os.Remove(h.path)
}
