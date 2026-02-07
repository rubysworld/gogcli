// Package markdown converts markdown text to Google Docs formatting requests.
package markdown

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"google.golang.org/api/docs/v1"
)

// Result contains the parsed markdown output for Google Docs.
type Result struct {
	// PlainText is the content with markdown syntax stripped
	PlainText string
	// Requests are the formatting requests to apply after inserting text
	Requests []*docs.Request
}

// Parse converts markdown content to plain text and Google Docs formatting requests.
// The baseIndex is the document index where the text will be inserted (usually 1 for new docs).
func Parse(content string, baseIndex int64) *Result {
	source := []byte(content)

	md := goldmark.New(
		goldmark.WithExtensions(extension.Strikethrough),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	doc := md.Parser().Parse(text.NewReader(source))

	w := &walker{
		source:    source,
		baseIndex: baseIndex,
		buf:       &bytes.Buffer{},
	}

	ast.Walk(doc, w.walk)

	// Ensure final newline
	plainText := w.buf.String()
	if !strings.HasSuffix(plainText, "\n") && plainText != "" {
		plainText += "\n"
	}

	return &Result{
		PlainText: plainText,
		Requests:  w.requests,
	}
}

type walker struct {
	source    []byte
	baseIndex int64
	buf       *bytes.Buffer
	requests  []*docs.Request

	// Track current paragraph for list bullets
	paragraphStart int64
	inList         bool
	listOrdered    bool
}

func (w *walker) walk(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch node := n.(type) {
	case *ast.Document:
		// Root node, just continue
		return ast.WalkContinue, nil

	case *ast.Heading:
		if entering {
			w.paragraphStart = w.currentIndex()
		} else {
			// Apply heading style
			w.buf.WriteString("\n")
			w.addHeadingStyle(w.paragraphStart, w.currentIndex(), node.Level)
		}
		return ast.WalkContinue, nil

	case *ast.Paragraph:
		if entering {
			w.paragraphStart = w.currentIndex()
		} else {
			w.buf.WriteString("\n")
			// If we're in a list, track the paragraph range for bullets
			if w.inList {
				w.addBulletRequest(w.paragraphStart, w.currentIndex(), w.listOrdered)
			}
		}
		return ast.WalkContinue, nil

	case *ast.List:
		if entering {
			w.inList = true
			w.listOrdered = node.IsOrdered()
		} else {
			w.inList = false
		}
		return ast.WalkContinue, nil

	case *ast.ListItem:
		// ListItem contains paragraphs, handled above
		return ast.WalkContinue, nil

	case *ast.TextBlock:
		if !entering {
			w.buf.WriteString("\n")
		}
		return ast.WalkContinue, nil

	case *ast.Text:
		if entering {
			start := w.currentIndex()
			segment := node.Segment
			w.buf.Write(segment.Value(w.source))
			end := w.currentIndex()

			// Apply any inline formatting from parent nodes
			w.applyInlineFormatting(n, start, end)

			if node.SoftLineBreak() {
				w.buf.WriteString(" ")
			}
			if node.HardLineBreak() {
				w.buf.WriteString("\n")
			}
		}
		return ast.WalkContinue, nil

	case *ast.String:
		if entering {
			w.buf.Write(node.Value)
		}
		return ast.WalkContinue, nil

	case *ast.Emphasis:
		// Formatting applied when we see the text node inside
		return ast.WalkContinue, nil

	case *extast.Strikethrough:
		return ast.WalkContinue, nil

	case *ast.Link:
		if entering {
			// We'll process children and add link formatting
		} else {
			// Link formatting is applied in applyInlineFormatting
		}
		return ast.WalkContinue, nil

	case *ast.AutoLink:
		if entering {
			start := w.currentIndex()
			url := string(node.URL(w.source))
			w.buf.WriteString(url)
			end := w.currentIndex()
			w.addLinkStyle(start, end, url)
		}
		return ast.WalkContinue, nil

	case *ast.CodeSpan:
		if entering {
			start := w.currentIndex()
			for i := 0; i < node.ChildCount(); i++ {
				child := node.FirstChild()
				for child != nil {
					if t, ok := child.(*ast.Text); ok {
						w.buf.Write(t.Segment.Value(w.source))
					}
					child = child.NextSibling()
				}
				break
			}
			end := w.currentIndex()
			w.addCodeStyle(start, end)
		}
		return ast.WalkSkipChildren, nil

	case *ast.FencedCodeBlock:
		if entering {
			start := w.currentIndex()
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				w.buf.Write(line.Value(w.source))
			}
			end := w.currentIndex()
			w.buf.WriteString("\n")
			w.addCodeStyle(start, end)
		}
		return ast.WalkContinue, nil

	case *ast.CodeBlock:
		if entering {
			start := w.currentIndex()
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				w.buf.Write(line.Value(w.source))
			}
			end := w.currentIndex()
			w.buf.WriteString("\n")
			w.addCodeStyle(start, end)
		}
		return ast.WalkContinue, nil

	case *ast.ThematicBreak:
		if entering {
			w.buf.WriteString("───────────────────────────────────────\n")
		}
		return ast.WalkContinue, nil

	case *ast.Blockquote:
		// Just render content, could add indentation later
		return ast.WalkContinue, nil

	case *ast.HTMLBlock, *ast.RawHTML:
		// Skip HTML
		return ast.WalkContinue, nil

	case *ast.Image:
		// Can't insert images via text, skip
		if entering {
			// Just write the alt text
			w.buf.WriteString("[")
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if t, ok := child.(*ast.Text); ok {
					w.buf.Write(t.Segment.Value(w.source))
				}
			}
			w.buf.WriteString("]")
		}
		return ast.WalkSkipChildren, nil
	}

	return ast.WalkContinue, nil
}

func (w *walker) currentIndex() int64 {
	return w.baseIndex + int64(w.buf.Len())
}

func (w *walker) applyInlineFormatting(n ast.Node, start, end int64) {
	if start >= end {
		return
	}

	// Walk up the tree to find formatting
	parent := n.Parent()
	var linkURL string

	for parent != nil {
		switch p := parent.(type) {
		case *ast.Emphasis:
			level := p.Level
			if level == 1 {
				w.addItalicStyle(start, end)
			} else if level >= 2 {
				w.addBoldStyle(start, end)
			}
		case *extast.Strikethrough:
			w.addStrikethroughStyle(start, end)
		case *ast.Link:
			linkURL = string(p.Destination)
		}
		parent = parent.Parent()
	}

	if linkURL != "" {
		w.addLinkStyle(start, end, linkURL)
	}
}

func (w *walker) addHeadingStyle(start, end int64, level int) {
	if start >= end {
		return
	}

	namedStyle := "HEADING_1"
	switch level {
	case 1:
		namedStyle = "HEADING_1"
	case 2:
		namedStyle = "HEADING_2"
	case 3:
		namedStyle = "HEADING_3"
	case 4:
		namedStyle = "HEADING_4"
	case 5:
		namedStyle = "HEADING_5"
	case 6:
		namedStyle = "HEADING_6"
	}

	w.requests = append(w.requests, &docs.Request{
		UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			ParagraphStyle: &docs.ParagraphStyle{
				NamedStyleType: namedStyle,
			},
			Fields: "namedStyleType",
		},
	})
}

func (w *walker) addBoldStyle(start, end int64) {
	if start >= end {
		return
	}
	w.requests = append(w.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				Bold: true,
			},
			Fields: "bold",
		},
	})
}

func (w *walker) addItalicStyle(start, end int64) {
	if start >= end {
		return
	}
	w.requests = append(w.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				Italic: true,
			},
			Fields: "italic",
		},
	})
}

func (w *walker) addStrikethroughStyle(start, end int64) {
	if start >= end {
		return
	}
	w.requests = append(w.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				Strikethrough: true,
			},
			Fields: "strikethrough",
		},
	})
}

func (w *walker) addCodeStyle(start, end int64) {
	if start >= end {
		return
	}
	w.requests = append(w.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				WeightedFontFamily: &docs.WeightedFontFamily{
					FontFamily: "Courier New",
				},
			},
			Fields: "weightedFontFamily",
		},
	})
}

func (w *walker) addLinkStyle(start, end int64, url string) {
	if start >= end || url == "" {
		return
	}
	w.requests = append(w.requests, &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				Link: &docs.Link{
					Url: url,
				},
			},
			Fields: "link",
		},
	})
}

func (w *walker) addBulletRequest(start, end int64, ordered bool) {
	if start >= end {
		return
	}

	preset := "BULLET_DISC_CIRCLE_SQUARE"
	if ordered {
		preset = "NUMBERED_DECIMAL_ALPHA_ROMAN"
	}

	w.requests = append(w.requests, &docs.Request{
		CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			BulletPreset: preset,
		},
	})
}
