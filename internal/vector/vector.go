// Package vector ports backend/src/lib/vector.ts (cosine similarity and
// min-max score normalization used by the hybrid retrieval ranking).
package vector

import "math"

// CosineSimilarity returns the cosine similarity of two equal-length vectors,
// or 0 when the inputs are empty, mismatched, or zero-norm. Mirrors
// cosineSimilarity in vector.ts, summing in index order for bit-identical
// results against the JS implementation.
func CosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return 0
	}

	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}

	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

// NormalizeScores min-max normalizes values into [0,1]. When all values are
// equal it returns 1 for every element, matching normalizeScores in vector.ts.
func NormalizeScores(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{}
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	out := make([]float64, len(values))
	if max == min {
		for i := range out {
			out[i] = 1
		}
		return out
	}

	for i, v := range values {
		out[i] = (v - min) / (max - min)
	}
	return out
}
