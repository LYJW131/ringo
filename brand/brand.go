// Package brand holds Ringo's mark, for the app to show where it greets a
// listener. The files beside it are written by go run ./icon (README.md).
package brand

import _ "embed"

// Mark is the mark in color, an SVG.
//
//go:embed mark.svg
var Mark []byte
