//go:build !darwin

package video

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

// Hardware reports whether covers are decoded by the machine's video
// decoder: only on the Mac (hardware_darwin.go). Windows' and Linux's are
// reached through APIs this repository can't test, so FFmpeg's decodes
// there (.work/motion/decision.md).
func Hardware() bool { return false }

func openHardware([]byte) (ui.VideoDecoder, error) {
	return nil, errors.New("video: no hardware decoder")
}
