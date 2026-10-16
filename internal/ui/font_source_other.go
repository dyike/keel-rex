//go:build !darwin

package ui

import "github.com/dyike/keel/ui/theme"

func prepareDarwinAppFonts(paths []string) (func() error, error) {
	set, err := theme.PrepareFontFiles(keepAppFont, paths...)
	if err != nil {
		return nil, err
	}
	return func() error { set.Use(); return nil }, nil
}
