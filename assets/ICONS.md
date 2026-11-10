# Interface icons

The packaged app icon is `rex-mascot-icon.png`: an original cartoon mint
T-rex holding a terminal prompt, with slate, ivory and muted lavender colors.
It was generated with the built-in image generation tool. The transparent
outer margin and rounded tile are included, so `keel.json` uses `icon_mask: none`.
The generation brief is preserved in `rex-mascot-icon.prompt.txt`.

The previous geometric K remains in `keel-icon.svg` and `keel-icon.png`.
`go run ./tools/appicon` regenerates that legacy mark only; it does not overwrite
the current mascot. Neither original app mark uses the upstream Rex artwork.

Source: https://github.com/egoist/gorex/tree/97d798d44bfff886e9aad6883186295d8bb97686/assets

Controls use gorex’s Lucide SVGs (24-unit viewBox, 2-unit rounded strokes).
Program badges use its Simple Icons SVGs and program tile colors.
Licenses are retained in `icons/LICENSE` and `brands/LICENSE.md`.

Lucide paths and rounded rectangles are expanded to explicit paths for the
Keel SVG renderer. The OpenAI brand path uses explicit commands and cubic
curves to preserve the internal gaps. Other brand paths retain their source
syntax. Geometry is unchanged. SVG views are embedded without a background,
cached by glyph, size and tint, and rasterized at physical pixel density.
