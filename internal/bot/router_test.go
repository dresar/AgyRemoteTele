package bot

import (
	"testing"
)

func TestNormalizeAccount(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"1", "agy01"},
		{"agy2", "agy02"},
		{"agy03", "agy03"},
		{"account4", "agy04"},
		{"AGY05", "agy05"},
		{"agy10", "agy10"},
		{"15", "agy15"},
		{"", ""},
	}

	for _, tc := range cases {
		got := normalizeAccount(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeAccount(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}
