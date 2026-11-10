package video

import (
	"io"
	"os"
	"testing"
)

// The Mac's own decoder, which plays covers there, gives the same frames
// in the same colors, and lets go of the file it reads from once closed.
func TestHardwarePlaysAndRewinds(t *testing.T) {
	d, err := openHardware(clip(t))
	if err != nil {
		t.Fatal(err)
	}
	playsAndRewinds(t, d)
	path := d.(*hardware).path
	if err := d.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("its file stayed: %v", err)
	}
}
