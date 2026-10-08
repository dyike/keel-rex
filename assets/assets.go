// Package assets embeds the interface icon resources.
package assets

import "embed"

//go:embed icons/*.svg brands/*.svg
var Icons embed.FS
