package util

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSplitCents(t *testing.T) {
	testCases := []struct {
		total     float64
		numSplits int
		expected  []float64
	}{
		{
			total:     0,
			numSplits: 1,
			expected:  []float64{0},
		},
		{
			total:     0,
			numSplits: 2,
			expected:  []float64{0, 0},
		},
		{
			total:     1,
			numSplits: 2,
			expected:  []float64{1, 0},
		},
		{
			total:     1,
			numSplits: 3,
			expected:  []float64{1, 0, 0},
		},
		{
			total:     2,
			numSplits: 3,
			expected:  []float64{1, 1, 0},
		},
		{
			total:     10,
			numSplits: 1,
			expected:  []float64{10},
		},
		{
			total:     10,
			numSplits: 2,
			expected:  []float64{5, 5},
		},
		{
			total:     11,
			numSplits: 2,
			expected:  []float64{6, 5},
		},
		{
			total:     9,
			numSplits: 3,
			expected:  []float64{3, 3, 3},
		},
		{
			total:     11,
			numSplits: 3,
			expected:  []float64{4, 4, 3},
		},
	}

	for i, tc := range testCases {
		result := make([]float64, len(tc.expected))
		SplitCents(tc.total, tc.numSplits, result)
		if !cmp.Equal(result, tc.expected) {
			t.Errorf("Test case %d total: %f, numSplits: %d, expected: %v, actual: %v", i, tc.total, tc.numSplits, tc.expected, result)
		}
	}
}

func TestSplitDollars(t *testing.T) {
	testCases := []struct {
		total     float64
		numSplits int
		expected  []float64
	}{
		{
			total:     0,
			numSplits: 1,
			expected:  []float64{0},
		},
		{
			total:     0,
			numSplits: 2,
			expected:  []float64{0, 0},
		},
		{
			total:     100,
			numSplits: 2,
			expected:  []float64{100, 0},
		},
		{
			total:     100,
			numSplits: 3,
			expected:  []float64{100, 0, 0},
		},
		{
			total:     200,
			numSplits: 3,
			expected:  []float64{100, 100, 0},
		},
		{
			total:     1000,
			numSplits: 1,
			expected:  []float64{1000},
		},
		{
			total:     1000,
			numSplits: 2,
			expected:  []float64{500, 500},
		},
		{
			total:     1100,
			numSplits: 2,
			expected:  []float64{600, 500},
		},
		{
			total:     900,
			numSplits: 3,
			expected:  []float64{300, 300, 300},
		},
		{
			total:     1100,
			numSplits: 3,
			expected:  []float64{400, 400, 300},
		},
	}

	for i, tc := range testCases {
		result := make([]float64, len(tc.expected))
		SplitDollars(tc.total, tc.numSplits, result)
		if !cmp.Equal(result, tc.expected) {
			t.Errorf("Test case %d total: %f, numSplits: %d, expected: %v, actual: %v", i, tc.total, tc.numSplits, tc.expected, result)
		}
	}
}

func TestFloorToNearest(t *testing.T) {
	testCases := []struct {
		in       float64
		nearest  int
		expected float64
	}{
		{0, 1, 0},
		{0, 10, 0},
		{0, 100, 0},
		{1, 1, 1},
		{1.5, 1, 1},
		{11.5, 1, 11},
		{1, 10, 0},
		{10, 10, 10},
		{10.5, 10, 10},
		{11.5, 10, 10},
		{15, 10, 10},
		{100, 10, 100},
		{100.5, 10, 100},
		{111.5, 10, 110},
		{101, 100, 100},
		{199, 100, 100},
	}

	for i, tc := range testCases {
		res := FloorToNearest(tc.in, tc.nearest)
		if res != tc.expected {
			t.Errorf("Test case %d in: %v, nearest: %d, expected: %v, actual: %v", i, tc.in, tc.nearest, tc.expected, res)
		}
	}
}
