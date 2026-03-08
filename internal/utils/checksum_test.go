package utils

import (
	"testing"
)

func TestCalChecksum(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected uint16
	}{
		{
			name:     "empty payload",
			payload:  []byte{},
			expected: 0xFFFF, // Initial value
		},
		{
			name:     "single byte 0x00",
			payload:  []byte{0x00},
			expected: 0xE1F0,
		},
		{
			name:     "single byte 0xFF",
			payload:  []byte{0xFF},
			expected: 0xFF00,
		},
		{
			name:     "ASCII string '123456789'",
			payload:  []byte("123456789"),
			expected: 0x29B1, // Standard CRC-16-CCITT test vector
		},
		{
			name:     "simple bytes",
			payload:  []byte{1, 2, 3, 4, 5},
			expected: 0x9304,
		},
		{
			name:     "repeated pattern",
			payload:  []byte{0xAA, 0xAA, 0xAA, 0xAA},
			expected: 0x9A55,
		},
		{
			name:     "all zeros",
			payload:  []byte{0, 0, 0, 0},
			expected: 0x84C0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalChecksum(tt.payload)
			if result != tt.expected {
				t.Errorf("CalChecksum(%v) = 0x%04X, want 0x%04X", tt.payload, result, tt.expected)
			}
			t.Logf("CRC16(%v) = 0x%04X", tt.payload, result)
		})
	}
}

func TestCalChecksumDeterministic(t *testing.T) {
	// Same input should always produce same output
	payload := []byte{1, 2, 3, 4, 5}

	results := make([]uint16, 100)
	for i := range results {
		results[i] = CalChecksum(payload)
	}

	for i, result := range results {
		if result != results[0] {
			t.Errorf("CalChecksum produced different results on iteration %d: %d vs %d", i, result, results[0])
		}
	}

	t.Logf("Deterministic test passed: all 100 calls returned 0x%04X", results[0])
}

func TestCalChecksumChangeDetection(t *testing.T) {
	base := []byte{1, 2, 3, 4, 5}
	baseCRC := CalChecksum(base)

	// Single bit change should produce different CRC
	modified := []byte{1, 2, 3, 4, 6} // Last byte changed from 5 to 6
	modifiedCRC := CalChecksum(modified)

	if baseCRC == modifiedCRC {
		t.Errorf("CRC collision: different data produced same CRC: 0x%04X", baseCRC)
	}

	t.Logf("Change detection: base=0x%04X, modified=0x%04X", baseCRC, modifiedCRC)
}

// Benchmark to measure performance
func BenchmarkCalChecksum(b *testing.B) {
	payload := []byte("Hello, World! This is a test payload for benchmarking.")

	for b.Loop() {
		CalChecksum(payload)
	}
}

func BenchmarkCalChecksumLarge(b *testing.B) {
	payload := make([]byte, 1024) // 1KB payload
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalChecksum(payload)
	}
}
