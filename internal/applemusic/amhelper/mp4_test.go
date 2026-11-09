package amhelper

import (
	"bytes"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"
)

// xorDecrypter "decrypts" by flipping every byte of the 16-byte-aligned
// prefix, as the white-box leaves a shorter tail as it is.
type xorDecrypter byte

func (x xorDecrypter) DecryptSample(s []byte, _ sampleEncryption) error {
	for i := range len(s) / 16 * 16 {
		s[i] ^= byte(x)
	}
	return nil
}

const timescale = 96000

// appleLike is an MP4 laid out as Apple's: one track, and fragments of 15
// seconds whose data counts from their moof, the second with its own sample
// description.
func appleLike(t *testing.T) ([]byte, [][]byte) {
	t.Helper()
	init := mp4.CreateEmptyInit()
	trak := init.AddEmptyTrack(timescale, "audio", "und")
	stsd := trak.Mdia.Minf.Stbl.Stsd
	stsd.AddChild(mp4.NewAudioSampleEntryBox("alac"))
	stsd.AddChild(mp4.NewAudioSampleEntryBox("alac"))
	var buf bytes.Buffer
	if err := init.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	var samples [][]byte
	var decodeTime uint64
	for n := range 2 {
		frag, err := mp4.CreateFragment(uint32(n+1), trak.Tkhd.TrackID)
		if err != nil {
			t.Fatal(err)
		}
		frag.Moof.Traf.Tfhd.Flags |= mp4.TfhdSampleDescriptionIndexPresentFlag
		frag.Moof.Traf.Tfhd.SampleDescriptionIndex = uint32(n + 1)
		for i := range 351 { // 15 s of 4096-sample packets
			data := bytes.Repeat([]byte{byte(i), byte(n)}, 20+i%40)
			samples = append(samples, data)
			frag.AddFullSample(mp4.FullSample{
				Sample:     mp4.NewSample(mp4.SyncSampleFlags, 4096, uint32(len(data)), 0),
				DecodeTime: decodeTime,
				Data:       data,
			})
			decodeTime += 4096
		}
		if err := frag.Encode(&buf); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes(), samples
}

func TestDecryptStreamSplitsAndDecrypts(t *testing.T) {
	in, plain := appleLike(t)
	var out bytes.Buffer
	flushes := 0
	keys := map[int]sampleDecrypter{1: xorDecrypter(0x5a), 2: xorDecrypter(0xa5)}
	err := decryptStream(bytes.NewReader(in), &out, func(desc int, _ bool) (sampleDecrypter, error) { return keys[desc], nil },
		streamOptions{seconds: 2.5, flush: func() { flushes++ }})
	if err != nil {
		t.Fatal(err)
	}
	f, err := mp4.DecodeFile(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	// The length, which the header of Apple's file leaves at 0.
	if mdhd := f.Init.Moov.Trak.Mdia.Mdhd; mdhd.Duration != uint64(2.5*float64(mdhd.Timescale)) {
		t.Errorf("mdhd says %d of %d, want 2.5 s", mdhd.Duration, mdhd.Timescale)
	}
	if mvhd := f.Init.Moov.Mvhd; mvhd.Duration != uint64(2.5*float64(mvhd.Timescale)) {
		t.Errorf("mvhd says %d of %d, want 2.5 s", mvhd.Duration, mvhd.Timescale)
	}
	if n := len(f.Init.Moov.Trak.Mdia.Minf.Stbl.Stsd.Children); n != 1 {
		t.Errorf("%d sample entries, want 1", n)
	}
	var got []mp4.FullSample
	frags := 0
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			frags++
			if frag.Moof.Traf.Tfhd.SampleDescriptionIndex > 1 {
				t.Errorf("fragment %d names sample description %d", frags, frag.Moof.Traf.Tfhd.SampleDescriptionIndex)
			}
			ss, err := frag.GetFullSamples(f.Init.Moov.Mvex.Trex)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, ss...)
		}
	}
	// A second of samples to a fragment: 24 of the 351 in each of Apple's.
	if want := 2 * 15; frags != want {
		t.Errorf("%d fragments, want %d", frags, want)
	}
	if flushes != frags+1 {
		t.Errorf("%d flushes, want one for the init and each fragment, %d", flushes, frags+1)
	}
	if len(got) != len(plain) {
		t.Fatalf("%d samples, want %d", len(got), len(plain))
	}
	for i, s := range got {
		want := bytes.Clone(plain[i])
		_ = keys[1+i/351].DecryptSample(want, sampleEncryption{})
		if !bytes.Equal(s.Data, want) {
			t.Fatalf("sample %d isn't decrypted with its fragment's key", i)
		}
		if s.DecodeTime != uint64(i)*4096 || s.Dur != 4096 {
			t.Fatalf("sample %d at %d for %d, want %d for 4096", i, s.DecodeTime, s.Dur, i*4096)
		}
	}
}

func TestDecryptStreamRejectsTruncatedMedia(t *testing.T) {
	in, _ := appleLike(t)
	err := decryptStream(bytes.NewReader(in[:len(in)-100]), &bytes.Buffer{},
		func(int, bool) (sampleDecrypter, error) { return xorDecrypter(1), nil }, streamOptions{flush: func() {}})
	if err == nil {
		t.Fatal("a cut-off file decrypted without an error")
	}
}

// A key that decrypts to noise is asked for again, and the samples it got
// wrong are decrypted anew before any of them is written.
func TestDecryptStreamReplacesAWrongKey(t *testing.T) {
	in, plain := appleLike(t)
	right := map[int]sampleDecrypter{1: xorDecrypter(0x5a), 2: xorDecrypter(0xa5)}
	asked := map[int]int{}
	keyOf := func(desc int, fresh bool) (sampleDecrypter, error) {
		if fresh {
			asked[desc]++
			return right[desc], nil
		}
		if desc == 2 {
			return xorDecrypter(0x33), nil // a stale template
		}
		return right[desc], nil
	}
	// The samples' second bytes are their fragment's number, 0 or 1, which
	// the right keys turn into 0x5a and 0xa4.
	isPlain := func(samples [][]byte) bool {
		for _, s := range samples {
			if s[1] != 0x5a && s[1] != 0xa4 {
				return false
			}
		}
		return true
	}
	var out bytes.Buffer
	if err := decryptStream(bytes.NewReader(in), &out, keyOf, streamOptions{check: isPlain, flush: func() {}}); err != nil {
		t.Fatal(err)
	}
	if asked[1] != 0 || asked[2] != 1 {
		t.Errorf("fresh keys asked for: %v, want description 2's once", asked)
	}
	f, err := mp4.DecodeFile(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	i := 0
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			ss, _ := frag.GetFullSamples(f.Init.Moov.Mvex.Trex)
			for _, s := range ss {
				want := bytes.Clone(plain[i])
				_ = right[1+i/351].DecryptSample(want, sampleEncryption{})
				if !bytes.Equal(s.Data, want) {
					t.Fatalf("sample %d isn't decrypted with the right key", i)
				}
				i++
			}
		}
	}
}

func TestDecryptStreamFailsWhenTheKeyStaysWrong(t *testing.T) {
	in, _ := appleLike(t)
	err := decryptStream(bytes.NewReader(in), &bytes.Buffer{},
		func(int, bool) (sampleDecrypter, error) { return xorDecrypter(0x33), nil },
		streamOptions{check: func([][]byte) bool { return false }, flush: func() {}})
	if err == nil {
		t.Fatal("noise was written as audio")
	}
}
