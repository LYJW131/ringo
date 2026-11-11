package audio

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// macOS plays through an Audio Queue on the default output, which follows
// it when it changes.

type audioStreamBasicDescription struct {
	sampleRate       float64
	formatID         uint32
	formatFlags      uint32
	bytesPerPacket   uint32
	framesPerPacket  uint32
	bytesPerFrame    uint32
	channelsPerFrame uint32
	bitsPerChannel   uint32
	reserved         uint32
}

type audioQueueBuffer struct {
	audioDataBytesCapacity uint32
	audioData              unsafe.Pointer
	audioDataByteSize      uint32
	userData               uintptr
	packetDescCapacity     uint32
	packetDescs            uintptr
	packetDescCount        uint32
}

const (
	kAudioFormatLinearPCM   = 0x6C70636D // 'lpcm'
	kAudioFormatFlagIsFloat = 1
	aqFrames                = 512 // per buffer: 10.7 ms, as many as hold ahead
)

var (
	aqLoad sync.Once
	aqErr  error

	audioQueueNewOutput      func(format *audioStreamBasicDescription, callback uintptr, user uintptr, runLoop, runLoopMode uintptr, flags uint32, out *uintptr) int32
	audioQueueAllocateBuffer func(q uintptr, size uint32, out **audioQueueBuffer) int32
	audioQueueEnqueueBuffer  func(q uintptr, b *audioQueueBuffer, n uint32, descs uintptr) int32
	audioQueueStart          func(q uintptr, at uintptr) int32
	audioQueueStop           func(q uintptr, immediate bool) int32
	audioQueuePause          func(q uintptr) int32
	audioQueueReset          func(q uintptr) int32
	audioQueueSetParameter   func(q uintptr, param uint32, value float32) int32
	audioQueueDispose        func(q uintptr, immediate bool) int32

	// aqCallback is created once; queues route buffers back to the device
	// open, through aqCurrent.
	aqCallback uintptr
	aqCurrent  atomic.Pointer[aqDevice]
)

func loadAudioToolbox() error {
	aqLoad.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			aqErr = err
			return
		}
		purego.RegisterLibFunc(&audioQueueNewOutput, lib, "AudioQueueNewOutput")
		purego.RegisterLibFunc(&audioQueueAllocateBuffer, lib, "AudioQueueAllocateBuffer")
		purego.RegisterLibFunc(&audioQueueEnqueueBuffer, lib, "AudioQueueEnqueueBuffer")
		purego.RegisterLibFunc(&audioQueueStart, lib, "AudioQueueStart")
		purego.RegisterLibFunc(&audioQueueStop, lib, "AudioQueueStop")
		purego.RegisterLibFunc(&audioQueuePause, lib, "AudioQueuePause")
		purego.RegisterLibFunc(&audioQueueReset, lib, "AudioQueueReset")
		purego.RegisterLibFunc(&audioQueueSetParameter, lib, "AudioQueueSetParameter")
		purego.RegisterLibFunc(&audioQueueDispose, lib, "AudioQueueDispose")
		// The queue's own thread calls it when it is done with a buffer.
		aqCallback = purego.NewCallback(func(user, q uintptr, b *audioQueueBuffer) {
			if d := aqCurrent.Load(); d != nil && d.q == q {
				select {
				case d.free <- b:
				default:
				}
			}
		})
	})
	return aqErr
}

type aqDevice struct {
	q       uintptr
	free    chan *audioQueueBuffer
	buffers int
	queued  int // buffers enqueued before the queue started
	started bool
}

func openDevice(ahead time.Duration, rate int) (device, int, error) {
	if err := loadAudioToolbox(); err != nil {
		return nil, 0, err
	}
	setDeviceRate(rate)
	desc := audioStreamBasicDescription{
		sampleRate:       float64(rate),
		formatID:         kAudioFormatLinearPCM,
		formatFlags:      kAudioFormatFlagIsFloat,
		bytesPerPacket:   channels * 4,
		framesPerPacket:  1,
		bytesPerFrame:    channels * 4,
		channelsPerFrame: channels,
		bitsPerChannel:   32,
	}
	n := max(2, int((int64(ahead)*int64(rate)/int64(time.Second)+aqFrames-1)/aqFrames))
	d := &aqDevice{free: make(chan *audioQueueBuffer, n), buffers: n}
	if st := audioQueueNewOutput(&desc, aqCallback, 0, 0, 0, 0, &d.q); st != 0 {
		return nil, 0, fmt.Errorf("audio: AudioQueueNewOutput failed (OSStatus %d)", st)
	}
	for range n {
		var b *audioQueueBuffer
		if st := audioQueueAllocateBuffer(d.q, aqFrames*channels*4, &b); st != 0 {
			audioQueueDispose(d.q, true)
			return nil, 0, fmt.Errorf("audio: AudioQueueAllocateBuffer failed (OSStatus %d)", st)
		}
		d.free <- b
	}
	aqCurrent.Store(d)
	return d, aqFrames, nil
}

func (d *aqDevice) write(samples []float32) error {
	var b *audioQueueBuffer
	select {
	case b = <-d.free:
	case <-time.After(time.Second):
		// The queue stopped taking buffers, as when its device went.
		return fmt.Errorf("audio: the audio queue stopped")
	}
	copy(unsafe.Slice((*float32)(b.audioData), len(samples)), samples)
	b.audioDataByteSize = uint32(len(samples) * 4)
	if st := audioQueueEnqueueBuffer(d.q, b, 0, 0); st != 0 {
		return fmt.Errorf("audio: AudioQueueEnqueueBuffer failed (OSStatus %d)", st)
	}
	if !d.started {
		// Start once every buffer holds something, not to begin with an
		// underrun.
		if d.queued++; d.queued == d.buffers {
			if st := audioQueueStart(d.q, 0); st != 0 {
				return fmt.Errorf("audio: AudioQueueStart failed (OSStatus %d)", st)
			}
			d.started = true
		}
	}
	return nil
}

// flushRamp is how long the queue takes to fade out before it stops at
// once: long enough not to click, short enough to feel like a button.
const flushRamp = 30 * time.Millisecond

const (
	kAudioQueueParamVolume         = 1
	kAudioQueueParamVolumeRampTime = 4
)

// flush fades the queue out, pauses it and takes back the buffers it held:
// it starts again, at its volume, once write has filled them.
func (d *aqDevice) flush() error {
	if d.started {
		audioQueueSetParameter(d.q, kAudioQueueParamVolumeRampTime, float32(flushRamp.Seconds()))
		audioQueueSetParameter(d.q, kAudioQueueParamVolume, 0)
		time.Sleep(flushRamp)
		if st := audioQueuePause(d.q); st != 0 {
			return fmt.Errorf("audio: AudioQueuePause failed (OSStatus %d)", st)
		}
	}
	if st := audioQueueReset(d.q); st != 0 {
		return fmt.Errorf("audio: AudioQueueReset failed (OSStatus %d)", st)
	}
	audioQueueSetParameter(d.q, kAudioQueueParamVolumeRampTime, 0)
	audioQueueSetParameter(d.q, kAudioQueueParamVolume, 1)
	d.started, d.queued = false, 0
	return nil
}

func (d *aqDevice) close() {
	aqCurrent.CompareAndSwap(d, nil)
	audioQueueStop(d.q, true)
	audioQueueDispose(d.q, true)
}

// The system's output device's rate, through Core Audio (devicerate.go).

type audioObjectPropertyAddress struct {
	selector, scope, element uint32
}

const (
	kAudioObjectSystemObject                        = 1
	kAudioObjectPropertyScopeGlobal                 = 0x676C6F62 // 'glob'
	kAudioHardwarePropertyDefaultOutputDevice       = 0x644F7574 // 'dOut'
	kAudioDevicePropertyNominalSampleRate           = 0x6E737274 // 'nsrt'
	kAudioDevicePropertyAvailableNominalSampleRates = 0x6E737223 // 'nsr#'
)

var (
	caLoad sync.Once
	caErr  error

	audioObjectGetPropertyDataSize func(id uint32, addr *audioObjectPropertyAddress, qualSize uint32, qual uintptr, size *uint32) int32
	audioObjectGetPropertyData     func(id uint32, addr *audioObjectPropertyAddress, qualSize uint32, qual uintptr, size *uint32, data unsafe.Pointer) int32
	audioObjectSetPropertyData     func(id uint32, addr *audioObjectPropertyAddress, qualSize uint32, qual uintptr, size uint32, data unsafe.Pointer) int32

	// The devices the output set the rate of: the rate each had, and the
	// one it set, under ratesMu.
	ratesMu  sync.Mutex
	hadRate  = map[uint32]float64{}
	gaveRate = map[uint32]float64{}
)

func loadCoreAudio() error {
	caLoad.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			caErr = err
			return
		}
		purego.RegisterLibFunc(&audioObjectGetPropertyDataSize, lib, "AudioObjectGetPropertyDataSize")
		purego.RegisterLibFunc(&audioObjectGetPropertyData, lib, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&audioObjectSetPropertyData, lib, "AudioObjectSetPropertyData")
	})
	return caErr
}

func globalAddress(selector uint32) *audioObjectPropertyAddress {
	return &audioObjectPropertyAddress{selector, kAudioObjectPropertyScopeGlobal, 0}
}

// defaultOutput returns the system's output device, or 0.
func defaultOutput() uint32 {
	var id uint32
	size := uint32(unsafe.Sizeof(id))
	if audioObjectGetPropertyData(kAudioObjectSystemObject, globalAddress(kAudioHardwarePropertyDefaultOutputDevice), 0, 0, &size, unsafe.Pointer(&id)) != 0 {
		return 0
	}
	return id
}

// nominalRate returns the device's rate, or 0.
func nominalRate(dev uint32) float64 {
	var rate float64
	size := uint32(unsafe.Sizeof(rate))
	if audioObjectGetPropertyData(dev, globalAddress(kAudioDevicePropertyNominalSampleRate), 0, 0, &size, unsafe.Pointer(&rate)) != 0 {
		return 0
	}
	return rate
}

// availableRates returns the ranges of rates the device takes.
func availableRates(dev uint32) [][2]float64 {
	addr := globalAddress(kAudioDevicePropertyAvailableNominalSampleRates)
	var size uint32
	if audioObjectGetPropertyDataSize(dev, addr, 0, 0, &size) != 0 || size < 16 {
		return nil
	}
	ranges := make([][2]float64, size/16)
	if audioObjectGetPropertyData(dev, addr, 0, 0, &size, unsafe.Pointer(&ranges[0])) != 0 {
		return nil
	}
	return ranges[:size/16]
}

// setNominalRate sets the device's rate and waits, a second at most, for it
// to run at it, as Core Audio changes it after it returns.
func setNominalRate(dev uint32, rate float64) bool {
	if audioObjectSetPropertyData(dev, globalAddress(kAudioDevicePropertyNominalSampleRate), 0, 0, uint32(unsafe.Sizeof(rate)), unsafe.Pointer(&rate)) != 0 {
		return false
	}
	for start := time.Now(); nominalRate(dev) != rate; time.Sleep(10 * time.Millisecond) {
		if time.Since(start) > time.Second {
			return false
		}
	}
	return true
}

// setDeviceRate sets the system's output device to the rate for music at
// rate, where it takes it. It never keeps the music from playing: a device
// that can't is left as it is, and the queue's rate converted.
func setDeviceRate(rate int) {
	// Tests that play leave the machine they run on as it is.
	if leaveDevice.Load() || testing.Testing() || loadCoreAudio() != nil {
		return
	}
	dev := defaultOutput()
	if dev == 0 {
		return
	}
	now := nominalRate(dev)
	want := float64(deviceRate(rate, availableRates(dev)))
	if want == 0 || now == 0 || now == want {
		return
	}
	ratesMu.Lock()
	defer ratesMu.Unlock()
	// The rate to put back is the one the device had before the output
	// first set it, or the one the listener set it to since.
	if gave, ok := gaveRate[dev]; !ok || now != gave {
		hadRate[dev] = now
	}
	// Noted before the device runs at it: one that takes longer than is
	// waited for is still put back.
	gaveRate[dev] = want
	setNominalRate(dev, want)
}

// outputDevice returns the system's output device, which the Audio Queue
// follows, or 0.
func outputDevice() uint32 {
	if loadCoreAudio() != nil {
		return 0
	}
	return defaultOutput()
}

func restoreDeviceRate() {
	if loadCoreAudio() != nil {
		return
	}
	ratesMu.Lock()
	defer ratesMu.Unlock()
	for dev, had := range hadRate {
		// A rate the listener set meanwhile is theirs.
		if gave, ok := gaveRate[dev]; ok && nominalRate(dev) == gave {
			setNominalRate(dev, had)
		}
		delete(hadRate, dev)
		delete(gaveRate, dev)
	}
}
