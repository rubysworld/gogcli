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

	// Should have 4 lines of text with tabs for nested items
	// Level 1: no tabs, Level 2: 1 tab, Level 3: 2 tabs
	expectedText := "Top level\n\tSub-bullet\n\t\tSub-sub-bullet\nBack to top\n"
	if result.PlainText != expectedText {
		t.Errorf("PlainText = %q, want %q", result.PlainText, expectedText)
	}

	// Should have bullet requests for each list item
	// No indentation requests - Google Docs uses the tabs for nesting
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

	// Should have NO indentation requests - nesting is handled by tabs
	if indentCount != 0 {
		t.Errorf("indent requests = %d, want 0 (nesting via tabs)", indentCount)
	}
}

func TestParseNestedOrderedList(t *testing.T) {
	content := `1. First
   1. Nested first
   2. Nested second
2. Second`

	result := Parse(content, 1)

	// Nested items should have 1 tab prefix
	expectedText := "First\n\tNested first\n\tNested second\nSecond\n"
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

func TestParseDeepNesting(t *testing.T) {
	content := `- Level 1
  - Level 2
    - Level 3
      - Level 4`

	result := Parse(content, 1)

	// Each level should have the appropriate number of tabs
	expectedText := "Level 1\n\tLevel 2\n\t\tLevel 3\n\t\t\tLevel 4\n"
	if result.PlainText != expectedText {
		t.Errorf("PlainText = %q, want %q", result.PlainText, expectedText)
	}

	// Verify no indentation requests
	for _, req := range result.Requests {
		if req.UpdateParagraphStyle != nil && req.UpdateParagraphStyle.ParagraphStyle.IndentStart != nil {
			t.Error("unexpected indentation request - nesting should use tabs")
		}
	}
}
