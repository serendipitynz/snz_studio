package vector

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-12
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name        string
		left, right []float64
		want        float64
	}{
		{"identical", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},
		{"orthogonal", []float64{1, 0}, []float64{0, 1}, 0},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1},
		{"empty left", []float64{}, []float64{1, 2}, 0},
		{"length mismatch", []float64{1, 2, 3}, []float64{1, 2}, 0},
		{"zero norm", []float64{0, 0}, []float64{1, 2}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CosineSimilarity(tt.left, tt.right); !almostEqual(got, tt.want) {
				t.Errorf("CosineSimilarity(%v,%v) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}

	// Known value: cos between (1,2,3) and (4,5,6) = 32 / (sqrt14*sqrt77).
	got := CosineSimilarity([]float64{1, 2, 3}, []float64{4, 5, 6})
	want := 32.0 / (math.Sqrt(14) * math.Sqrt(77))
	if !almostEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeScores(t *testing.T) {
	if got := NormalizeScores(nil); len(got) != 0 {
		t.Errorf("empty: got %v", got)
	}

	// All equal -> all 1.
	for _, v := range NormalizeScores([]float64{5, 5, 5}) {
		if v != 1 {
			t.Errorf("all-equal: got %v, want 1", v)
		}
	}

	got := NormalizeScores([]float64{0, 5, 10})
	want := []float64{0, 0.5, 1}
	for i := range want {
		if !almostEqual(got[i], want[i]) {
			t.Errorf("index %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
