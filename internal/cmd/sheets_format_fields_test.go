package cmd

import (
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestApplyForceSendFields_TextFormatBold(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, "userEnteredFormat.textFormat.bold"); err != nil {
		t.Fatalf("applyForceSendFields: %v", err)
	}
	if format.TextFormat == nil {
		t.Fatalf("expected textFormat to be allocated")
	}
	if !hasString(format.TextFormat.ForceSendFields, "Bold") {
		t.Fatalf("expected Bold to be force-sent, got %#v", format.TextFormat.ForceSendFields)
	}
}

func TestApplyForceSendFields_UnknownField(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, "userEnteredFormat.nope"); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
