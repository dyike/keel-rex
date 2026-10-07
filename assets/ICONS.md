# Interface icons

Source: https://github.com/egoist/gorex/tree/97d798d44bfff886e9aad6883186295d8bb97686/assets

Controls use gorex’s Lucide SVGs (24-unit viewBox, 2-unit rounded strokes).
Program badges use its Simple Icons SVGs and program tile colors.
Licenses are retained in `icons/LICENSE` and `brands/LICENSE.md`.

Lucide paths and rounded rectangles are expanded to explicit paths for the
Keel SVG renderer. The OpenAI brand path uses explicit commands and cubic
curves to preserve the internal gaps. Other brand paths retain their source
syntax. Geometry is unchanged. SVG views are embedded without a background,
cached by glyph, size and tint, and rasterized at physical pixel density.
