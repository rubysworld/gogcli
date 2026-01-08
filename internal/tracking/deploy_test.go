package tracking

import "testing"

func TestSanitizeWorkerName(t *testing.T) {
	cases := map[string]string{
		"Test@Example.com": "test-example-com",
		" gog--tracker ":   "gog-tracker",
		"___":              "",
	}
	for input, want := range cases {
		if got := SanitizeWorkerName(input); got != want {
			t.Fatalf("SanitizeWorkerName(%q) = %q, want %q", input, got, want)
		}
	}

	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := SanitizeWorkerName(long); len(got) != 63 {
		t.Fatalf("expected max length 63, got %d (%q)", len(got), got)
	}
}

func TestParseDatabaseID(t *testing.T) {
	cases := map[string]string{
		`database_id = "abc-123"`:   "abc-123",
		`database_id: abc-123`:      "abc-123",
		`Database ID: abc-123`:      "abc-123",
		`database_id: "xyz-789"`:    "xyz-789",
		`Database ID: 12345`:        "12345",
		`database_id = "with-dash"`: "with-dash",
	}
	for input, want := range cases {
		if got := parseDatabaseID(input); got != want {
			t.Fatalf("parseDatabaseID(%q) = %q, want %q", input, got, want)
		}
	}

	if got := parseDatabaseID("nope"); got != "" {
		t.Fatalf("expected empty id, got %q", got)
	}
}
