package markdown

import (
	"testing"
)

func TestParseNestedBullets(t *testing.T) {
	content := `- Top level
  - Sub-bullet
    - Sub-sub-bullet
- Back to top`

	result := Parse(content, 1)

	// Should have 4 lines of text
	expectedText := "Top level\nSub-bullet\nSub-sub-bullet\nBack to top\n"
	if result.PlainText != expectedText {
		t.Errorf("PlainText = %q, want %q", result.PlainText, expectedText)
	}

	// Should have bullet requests for each list item
	// Plus indentation requests for nested items (level 2 and 3)
	bulletCount := 0
	indentCount := 0
	for _, req := range result.Requests {
		if req.CreateParagraphBullets != nil {
			bulletCount++
		}
		if req.UpdateParagraphStyle != nil && req.UpdateParagraphStyle.ParagraphStyle.IndentStart != nil {
			indentCount++
		}
	}

	if bulletCount != 4 {
		t.Errorf("bullet requests = %d, want 4", bulletCount)
	}

	// Level 2 and 3 items should have indentation (2 items)
	if indentCount != 2 {
		t.Errorf("indent requests = %d, want 2", indentCount)
	}
}

func TestParseNestedOrderedList(t *testing.T) {
	content := `1. First
   1. Nested first
   2. Nested second
2. Second`

	result := Parse(content, 1)

	expectedText := "First\nNested first\nNested second\nSecond\n"
	if result.PlainText != expectedText {
		t.Errorf("PlainText = %q, want %q", result.PlainText, expectedText)
	}

	// Check that we have bullet requests with ordered preset
	orderedCount := 0
	for _, req := range result.Requests {
		if req.CreateParagraphBullets != nil {
			if req.CreateParagraphBullets.BulletPreset == "NUMBERED_DECIMAL_ALPHA_ROMAN" {
				orderedCount++
			}
		}
	}

	if orderedCount != 4 {
		t.Errorf("ordered bullet requests = %d, want 4", orderedCount)
	}
}

func TestParseMixedNestedLists(t *testing.T) {
	content := `- Unordered top
  1. Ordered nested
  2. Another ordered
- Back to unordered`

	result := Parse(content, 1)

	unorderedCount := 0
	orderedCount := 0
	for _, req := range result.Requests {
		if req.CreateParagraphBullets != nil {
			if req.CreateParagraphBullets.BulletPreset == "NUMBERED_DECIMAL_ALPHA_ROMAN" {
				orderedCount++
			} else {
				unorderedCount++
			}
		}
	}

	if unorderedCount != 2 {
		t.Errorf("unordered bullet requests = %d, want 2", unorderedCount)
	}
	if orderedCount != 2 {
		t.Errorf("ordered bullet requests = %d, want 2", orderedCount)
	}
}
