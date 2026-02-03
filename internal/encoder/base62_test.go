package encoder

import "testing"

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"nine", 9, "9"},
		{"ten becomes 'a'", 10, "a"},
		{"thirty-five becomes 'z'", 35, "z"},
		{"thirty-six becomes 'A'", 36, "A"},
		{"sixty-one becomes 'Z'", 61, "Z"},
		{"sixty-two becomes '10'", 62, "10"},
		{"large number", 12345, "3d7"},
		{"million", 1000000, "4c92"},
		{"realistic ID", 123456789, "8m0Kx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Encode(%d) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{"zero", "0", 0},
		{"single digit", "5", 5},
		{"letter 'a' is 10", "a", 10},
		{"letter 'z' is 35", "z", 35},
		{"letter 'A' is 36", "A", 36},
		{"letter 'Z' is 61", "Z", 61},
		{"'10' is 62", "10", 62},
		{"large number", "3d7", 12345},
		{"million", "4c92", 1000000},
		{"realistic ID", "8m0Kx", 123456789},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Decode(tt.input)
			if result != tt.expected {
				t.Errorf("Decode(%s) = %d; want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// This is the most important test!
	// Whatever we encode, we should be able to decode back
	testNumbers := []uint64{0, 1, 10, 61, 62, 100, 1000, 12345, 999999, 123456789}

	for _, num := range testNumbers {
		encoded := Encode(num)
		decoded := Decode(encoded)

		if decoded != num {
			t.Errorf("Round trip failed: %d -> %s -> %d", num, encoded, decoded)
		}
	}
}

func TestEncodedLength(t *testing.T) {
	// Let's verify our math about capacity
	// 6 characters should handle up to 62^6 - 1 = 56,800,235,583

	tests := []struct {
		input       uint64
		maxLength   int
		description string
	}{
		{61, 1, "max 1-char"},
		{62*62 - 1, 2, "max 2-char"},       // 3843
		{62*62*62 - 1, 3, "max 3-char"},    // 238,327
		{62*62*62*62 - 1, 4, "max 4-char"}, // 14,776,335
		{1000000, 4, "1 million fits in 4"},
		{56800235583, 6, "max 6-char capacity"}, // 62^6 - 1
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			encoded := Encode(tt.input)
			if len(encoded) > tt.maxLength {
				t.Errorf("Encode(%d) = %s (len=%d); want max length %d",
					tt.input, encoded, len(encoded), tt.maxLength)
			}
		})
	}
}
