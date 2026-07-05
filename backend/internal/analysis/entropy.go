// entropy.go — Shannon entropy over a byte slice.
//
// Formula:
//
//	H = -Σ  p_i * log2(p_i)   over all bytes that appear in `data`,
//	where p_i = count(byte_i) / len(data).
//
// Range: 0.0 (all bytes identical) to 8.0 (uniformly distributed
// bytes, log2(256)). For a payload of at least a few hundred bytes,
// values above ~7.5 are an empirical signal of "looks encrypted or
// compressed"; values below ~5 indicate structured / repetitive
// content.
//
// COMPLEXITY
//
// One pass over `data` for the histogram, one pass over the 256
// buckets to convert counts to bit-shares, then a final accumulate
// for the sum. O(N) time, O(1) extra memory. Allocation-free in the
// hot path.
//
// PRIVACY
//
// Pure function on `[]byte`. Returns a single float64; never writes
// to log, never copies, never retains the slice.
package analysis

import "math"

// ShannonEntropy returns the Shannon entropy (in bits per byte) of
// the input. Returns 0.0 for empty input — the formula is undefined
// at N=0, and "we saw no data" maps to "no signal" which scores the
// same as "all bytes identical" for our purposes.
func ShannonEntropy(data []byte) float64 {
	n := len(data)
	if n == 0 {
		return 0
	}

	// Histogram on the stack. [256]int is 2 KiB on amd64 — well
	// inside the Go runtime's "no escape" budget for a small slice
	// header + array literal.
	var counts [256]int64
	for _, b := range data {
		counts[b]++
	}

	// Convert counts to bit-shares and accumulate. log2 is built
	// in; we avoid pulling in a precomputed log table to keep the
	// file dependency-free (this file is on the cold start-up path
	// of every session, no need to optimise microseconds).
	var sum float64
	invN := 1.0 / float64(n)
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) * invN
		sum -= p * math.Log2(p)
	}
	return sum
}
