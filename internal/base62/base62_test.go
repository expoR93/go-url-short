package base62

import (
	"math"
	"testing"
)

func TestEncode(t *testing.T) {
	testCases := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"Zero Value", 0, "0"},                        // Smallest boundary
		{"Single Digit", 10, "A"},                     // Tenth char in alphabet
		{"Alphabet Boundary", 61, "z"},                // Last char in alphabet
		{"Two Digit Result", 62, "10"},                // First wrap-around (62nd index)
		{"Large ID", 123456789, "8M0kX"},              // Typical database ID scale
		{"Max Uint64", math.MaxUint64, "LygHa16AHYF"}, // Stress testing capacity
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Encode(tc.input)
			if got != tc.expected {
				t.Errorf("Encode(%d) = %v; expected %v", tc.input, got, tc.expected)
			}
		})
	}
}

// TestDecodeErrors tests the "Negative" path where input is invalid.
func TestDecodeErrors(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"Special Character", "abc-123"}, // Contains '-' not in alphabet
		{"Space", "a b"},                 // Contains whitespace
		{"Emoji", "a🚀b"},                 // Non-ASCII characters
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.input)
			if err == nil {
				t.Errorf("Decode(%s) expected error for invalid character, but got nil", tc.input)
			}

			if err != nil && err.Error() != "Invalid character" {
				t.Errorf("Unexpected error message: %v", err)
			}
		})
	}
}

// TestIdentityProperty is a "Round-trip" test. It ensures that if we encode then decode,
// we get exactly what we started with.
func TestIdentityProperty(t *testing.T) {
	// Mixed range of values to ensure round-trip consistency
	values := []uint64{1, 55, 1024, 999999, 4503599627370496, math.MaxUint64}

	for _, val := range values {
		encoded := Encode(val)
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Identity failed: Decode returned error for %s: %v", encoded, err)
		}

		if decoded != val {
			t.Errorf("Identity failed: Start: %d -> Encoded: %s -> Decoded: %d", val, encoded, decoded)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	// b.N is adjusted by the Go runtime until the benchmark
	// results are statistically significant.
	for i := 0; i < b.N; i++ {
		Encode(math.MaxUint64)
	}
}

func BenchmarkDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Decode("Lyg78DxNoYJ")
	}
}
