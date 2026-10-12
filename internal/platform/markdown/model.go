package markdown

import "github.com/egoist/mygo/ui"

type kind uint8

const (
	kParagraph kind = iota
	kHeading
	kList
	kItem
	kCode
	kSuggestion
	kQuote
	kAlert
	kTable
	kRule
	kDetails
	kImages
	kVideo
)

// block is one block of a parsed text.
type block struct {
	kind kind
	// level is a heading's level or an ordered list's first number.
	level   int
	ordered bool
	// task and checked are a list item's checkbox.
	task, checked bool
	// runs are a paragraph's or heading's text, or a <details>' summary.
	runs     []run
	children []block

	// lang, code and spans are a code block's: its language, its text
	// for Copy and its highlighted spans.
	lang  string
	code  string
	spans []ui.Span
	// removed and added are a suggestion's lines.
	removed, added []string
	// alert is an alert's type: NOTE, TIP, IMPORTANT, WARNING or CAUTION.
	alert string
	// rows are a table's cells, the header first; align is per column.
	rows  [][][]run
	align []ui.Align
	// images are an images block's.
	images []picture
	// video is a video block's link.
	video string
}

type style uint8

const (
	bold style = 1 << iota
	italic
	strike
	code
	small
)

// run is a stretch of text in one style; url makes it a link.
type run struct {
	text  string
	style style
	url   string
	// path is the target Options.Paths gave, opened by Options.Open.
	path string
	// img marks the stand-in of an image in a sentence, video a <video>.
	img, video bool
}

type picture struct {
	src, alt, href string
	// width is the width the text asks for, in DIPs; 0 is the image's own.
	width float32
}
