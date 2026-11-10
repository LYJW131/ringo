package amhelper

// How loud a song is, as Apple measured it. Every variant of a song carries
// it in its MP4's header, in the loudness box of ISO/IEC 14496-12 (12.2.7):
// moov/trak/udta/ludt, with a tlou for the track and an alou for its album.
// The fields are those of MPEG-D DRC (ISO/IEC 23003-4). Each variant has
// its own: the stereo of a song, the binaural and the downmix of its Dolby
// Atmos version measure apart, and the stereo ones, AAC or ALAC, alike.

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
)

// Loudness is a track's loudness and peak, as its tlou says, and its
// album's, as its alou does.
type Loudness struct {
	// LKFS is its integrated loudness, by ITU-R BS.1770 (DRC's program
	// loudness); 0 where it doesn't say.
	LKFS float64 `json:"lkfs,omitempty"`
	// Peak is its true peak, or its sample peak where it says only that,
	// linear: 1 is full scale. 0 where it says neither.
	Peak float64 `json:"peak,omitempty"`
	// Album is the loudness and peak of the whole album the track is on,
	// measured as one, for playing the album through without evening its
	// songs out against each other. It's never nil where the header was
	// read, and a zero Loudness where it says none: one kept from before
	// it was read is nil, and worked out again.
	Album *Loudness `json:"album,omitempty"`
}

// programLoudness is the measurement of DRC's method_definition 1.
const programLoudness = 1

// readLoudness is the first track's loudness, and its album's, from the
// MP4's header: its ftyp and moov, read as raw boxes, as mp4ff leaves ludt
// unread. ok is false where it has no loudness of the track.
func readLoudness(head []byte) (l Loudness, ok bool) {
	l.Album = &Loudness{}
	b := head
	for _, typ := range []string{"moov", "trak", "udta", "ludt"} {
		if b, ok = childBox(b, typ); !ok {
			return l, false
		}
	}
	if alou, found := childBox(b, "alou"); found {
		*l.Album, _ = parseTlou(alou)
	}
	tlou, found := childBox(b, "tlou")
	if !found {
		return l, false
	}
	track, ok := parseTlou(tlou)
	l.LKFS, l.Peak = track.LKFS, track.Peak
	return l, ok
}

// childBox is the body of the first box of the type among boxes.
func childBox(boxes []byte, typ string) ([]byte, bool) {
	for len(boxes) >= 8 {
		size := binary.BigEndian.Uint32(boxes)
		if size < 8 || int64(size) > int64(len(boxes)) {
			return nil, false
		}
		if string(boxes[4:8]) == typ {
			return boxes[8:size], true
		}
		boxes = boxes[size:]
	}
	return nil, false
}

// parseTlou reads a tlou's body, or an alou's, both a LoudnessBaseBox:
// of its sets, the one
// for the audio as it is, with no downmix, DRC or EQ applied.
func parseTlou(b []byte) (Loudness, bool) {
	if len(b) < 4 {
		return Loudness{}, false
	}
	version := b[0]
	b = b[4:]
	sets := 1
	if version >= 1 {
		if len(b) < 1 {
			return Loudness{}, false
		}
		sets = int(b[0] & 0x3f)
		b = b[1:]
	}
	for range sets {
		eq := 0
		if version >= 1 {
			if len(b) < 1 {
				return Loudness{}, false
			}
			eq = int(b[0] & 0x3f)
			b = b[1:]
		}
		if len(b) < 7 {
			return Loudness{}, false
		}
		ids := binary.BigEndian.Uint16(b)
		downmix, drc := (ids>>6)&0x7f, ids&0x3f
		peaks := uint32(b[2])<<16 | uint32(b[3])<<8 | uint32(b[4])
		samplePeak, truePeak := peaks>>12, peaks&0xfff
		count := int(b[6])
		b = b[7:]
		if len(b) < 3*count {
			return Loudness{}, false
		}
		var l Loudness
		for i := range count {
			if m := b[3*i:]; m[0] == programLoudness {
				l.LKFS = -57.75 + 0.25*float64(m[1])
			}
		}
		b = b[3*count:]
		if eq != 0 || downmix != 0 || drc != 0 {
			continue
		}
		// A level of 0 is none; the others count down from 20 dB, in
		// 32nds of one.
		switch {
		case truePeak != 0:
			l.Peak = math.Pow(10, (20-float64(truePeak)/32)/20)
		case samplePeak != 0:
			l.Peak = math.Pow(10, (20-float64(samplePeak)/32)/20)
		}
		return l, l.LKFS != 0 || l.Peak != 0
	}
	return Loudness{}, false
}

// headMost is the most of a file's start read for its header: Apple's are
// about a kilobyte.
const headMost = 64 << 10

// readHead reads an MP4's top-level boxes from r up to the end of its moov.
func readHead(r io.Reader) ([]byte, error) {
	var head []byte
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return head, err
		}
		size := binary.BigEndian.Uint32(hdr[:])
		if size < 8 || len(head)+int(size) > headMost {
			return head, fmt.Errorf("a %q box of %d bytes where the header should be", hdr[4:], size)
		}
		box := make([]byte, size)
		copy(box, hdr[:])
		if _, err := io.ReadFull(r, box[8:]); err != nil {
			return head, err
		}
		head = append(head, box...)
		if string(hdr[4:]) == "moov" {
			return head, nil
		}
	}
}

// fetchLoudness reads the loudness from the start of the media file, as
// much of it as the playlist's EXT-X-MAP says is the header, or headMost.
// A file that says none comes back as a zero Loudness: it was looked for.
func fetchLoudness(ctx context.Context, client *http.Client, mediaURL string, headLen int) (*Loudness, error) {
	if headLen <= 0 || headLen > headMost {
		headLen = headMost
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", headLen-1))
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent && res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media header: HTTP %d", res.StatusCode)
	}
	head, err := readHead(io.LimitReader(res.Body, int64(headLen)))
	if err != nil {
		return nil, err
	}
	l, _ := readLoudness(head)
	return &l, nil
}
