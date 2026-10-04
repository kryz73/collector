package youtube

import "testing"

func TestParseISODuration(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"PT1H2M30S", 3750},
		{"PT4M15S", 255},
		{"PT45S", 45},
		{"PT1H", 3600},
		{"PT2H30M", 9000},
		{"P1DT2H", 93600},
		{"PT0S", 0},
		{"", 0},
		{"invalid", 0},
		{"PT10M", 600},
	}

	for _, tt := range tests {
		got := ParseISODuration(tt.input)
		if got != tt.expected {
			t.Errorf("ParseISODuration(%q) = %d, expected %d", tt.input, got, tt.expected)
		}
	}
}
