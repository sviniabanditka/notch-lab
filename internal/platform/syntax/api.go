// Package syntax colors source code: chroma's token types mapped once to
// ui colors, shared by every view that highlights code.
package syntax

import (
	"maps"
	"slices"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/egoist/mygo/ui"
)

// Palette maps chroma's token types to colors. It is read-only once made,
// so any goroutine may use it.
type Palette struct {
	index map[chroma.TokenType]uint16
	// Colors are the palette's colors by index; Colors[0] is the plain
	// text color, which Of returns for tokens the style does not color.
	Colors []ui.Color
}

// NewPalette maps the token types of chroma style styleName; plain is the
// color of text the style leaves uncolored.
func NewPalette(styleName string, plain ui.Color) *Palette {
	st := styles.Get(styleName)
	text := st.Get(chroma.Text).Colour
	p := &Palette{index: map[chroma.TokenType]uint16{}, Colors: []ui.Color{plain}}
	byRGB := map[chroma.Colour]uint16{}
	for _, tt := range slices.Sorted(maps.Keys(chroma.StandardTypes)) { // the same order each run
		c := st.Get(tt).Colour
		if !c.IsSet() || c == text {
			continue
		}
		i, ok := byRGB[c]
		if !ok {
			i = uint16(len(p.Colors))
			byRGB[c] = i
			p.Colors = append(p.Colors, ui.RGB(c.Red(), c.Green(), c.Blue()))
		}
		p.index[tt] = i
	}
	return p
}

// Of is the index in Colors of tt's color, falling back to its
// subcategory's and category's; 0 is plain text.
func (p *Palette) Of(tt chroma.TokenType) uint16 {
	if i, ok := p.index[tt]; ok {
		return i
	}
	if i, ok := p.index[tt.SubCategory()]; ok {
		return i
	}
	return p.index[tt.Category()]
}
