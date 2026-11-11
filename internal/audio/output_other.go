//go:build !darwin && !(linux && (amd64 || arm64)) && !(windows && (amd64 || arm64))

package audio

import "time"

func openDevice(ahead time.Duration, rate int) (device, int, error) { return nil, 0, errUnsupported }

// restoreDeviceRate does nothing: only macOS sets the device's rate.
func restoreDeviceRate() {}
