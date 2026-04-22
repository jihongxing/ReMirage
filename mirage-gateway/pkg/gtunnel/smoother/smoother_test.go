package smoother

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genMarkovMatrix generates a valid N×N Markov matrix (rows sum to 1, elements in [0,1]).
func genMarkovMatrix(n int) *rapid.Generator[MarkovMatrix] {
	return rapid.Custom(func(t *rapid.T) MarkovMatrix {
		m := make(MarkovMatrix, n)
		for i := 0; i < n; i++ {
			row := make([]float64, n)
			sum := 0.0
			for j := 0; j < n; j++ {
				row[j] = rapid.Float64Range(0.01, 1.0).Draw(t, "cell")
				sum += row[j]
			}
			for j := 0; j < n; j++ {
				row[j] /= sum
			}
			m[i] = row
		}
		return m
	})
}

// TestProperty8_MarkovInterpolation verifies linear interpolation correctness.
// **Validates: Requirements 6.3, 6.4, 6.5**
func TestProperty8_MarkovInterpolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 5).Draw(t, "n")
		mOld := genMarkovMatrix(n).Draw(t, "mOld")
		mNew := genMarkovMatrix(n).Draw(t, "mNew")
		alpha := rapid.Float64Range(0, 1).Draw(t, "alpha")

		result := InterpolateMatrixAt(mOld, mNew, alpha)

		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				expected := (1-alpha)*mOld[i][j] + alpha*mNew[i][j]
				if math.Abs(result[i][j]-expected) > 1e-9 {
					t.Fatalf("M[%d][%d]: got %f, want %f", i, j, result[i][j], expected)
				}
			}
		}

		// Boundary: alpha=0 → M_old, alpha=1 → M_new
		r0 := InterpolateMatrixAt(mOld, mNew, 0)
		r1 := InterpolateMatrixAt(mOld, mNew, 1)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if math.Abs(r0[i][j]-mOld[i][j]) > 1e-9 {
					t.Fatalf("alpha=0: M[%d][%d] != M_old", i, j)
				}
				if math.Abs(r1[i][j]-mNew[i][j]) > 1e-9 {
					t.Fatalf("alpha=1: M[%d][%d] != M_new", i, j)
				}
			}
		}

		// Distribution interpolation
		oldDist := PacketSizeDistribution{
			Mean:   rapid.Float64Range(100, 1500).Draw(t, "oldMean"),
			StdDev: rapid.Float64Range(10, 200).Draw(t, "oldStdDev"),
		}
		newDist := PacketSizeDistribution{
			Mean:   rapid.Float64Range(100, 1500).Draw(t, "newMean"),
			StdDev: rapid.Float64Range(10, 200).Draw(t, "newStdDev"),
		}
		rd := InterpolateDistAt(oldDist, newDist, alpha)
		expectedMean := (1-alpha)*oldDist.Mean + alpha*newDist.Mean
		expectedStdDev := (1-alpha)*oldDist.StdDev + alpha*newDist.StdDev
		if math.Abs(rd.Mean-expectedMean) > 1e-9 {
			t.Fatalf("mean: got %f, want %f", rd.Mean, expectedMean)
		}
		if math.Abs(rd.StdDev-expectedStdDev) > 1e-9 {
			t.Fatalf("stddev: got %f, want %f", rd.StdDev, expectedStdDev)
		}
	})
}

// TestProperty9_TransitionInterruptContinuity verifies that interrupting a transition
// uses the current interpolated state as the new start.
// **Validates: Requirements 6.6**
func TestProperty9_TransitionInterruptContinuity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 4).Draw(t, "n")
		mOld := genMarkovMatrix(n).Draw(t, "mOld")
		mNew := genMarkovMatrix(n).Draw(t, "mNew")
		mNew2 := genMarkovMatrix(n).Draw(t, "mNew2")

		smoother := NewTransitionSmoother(nil)

		// Start first transition with long duration
		dur := 10 * time.Second
		err := smoother.BeginTransition(mOld, mNew,
			PacketSizeDistribution{Mean: 500, StdDev: 50},
			PacketSizeDistribution{Mean: 1000, StdDev: 100},
			dur)
		if err != nil {
			t.Fatalf("BeginTransition: %v", err)
		}

		// Capture current state mid-transition
		time.Sleep(5 * time.Millisecond)
		midMatrix := smoother.CurrentMatrix()
		midDist := smoother.CurrentDistribution()

		// Start new transition (interrupt) — oldMatrix/oldDist are ignored when transitioning
		err = smoother.BeginTransition(mOld, mNew2,
			PacketSizeDistribution{Mean: 500, StdDev: 50}, PacketSizeDistribution{Mean: 1200, StdDev: 120},
			dur)
		if err != nil {
			t.Fatalf("BeginTransition (interrupt): %v", err)
		}

		// The new start should be close to the mid-transition state
		newStart := smoother.CurrentMatrix() // alpha ≈ 0 since we just started
		// Since alpha is very small (just started), newStart ≈ oldMatrix of new transition ≈ midMatrix
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				// Allow tolerance since time passes between captures
				if math.Abs(newStart[i][j]-midMatrix[i][j]) > 0.1 {
					t.Fatalf("M[%d][%d]: new start %f far from mid %f", i, j, newStart[i][j], midMatrix[i][j])
				}
			}
		}

		newStartDist := smoother.CurrentDistribution()
		if math.Abs(newStartDist.Mean-midDist.Mean) > 50 {
			t.Fatalf("mean: new start %f far from mid %f", newStartDist.Mean, midDist.Mean)
		}
	})
}

// TestSmoother_DefaultDuration verifies duration=0 uses default 3000ms.
func TestSmoother_DefaultDuration(t *testing.T) {
	s := NewTransitionSmoother(nil)
	m := MarkovMatrix{{0.5, 0.5}, {0.3, 0.7}}
	err := s.BeginTransition(m, m,
		PacketSizeDistribution{Mean: 500, StdDev: 50},
		PacketSizeDistribution{Mean: 500, StdDev: 50},
		0)
	if err != nil {
		t.Fatalf("BeginTransition: %v", err)
	}
	if s.duration != DefaultTransitionDuration {
		t.Fatalf("expected default duration %v, got %v", DefaultTransitionDuration, s.duration)
	}
}

// TestSmoother_DimensionMismatch verifies mismatched matrix dimensions return error.
func TestSmoother_DimensionMismatch(t *testing.T) {
	s := NewTransitionSmoother(nil)
	m2 := MarkovMatrix{{0.5, 0.5}, {0.3, 0.7}}
	m3 := MarkovMatrix{{0.3, 0.3, 0.4}, {0.2, 0.5, 0.3}, {0.1, 0.1, 0.8}}
	err := s.BeginTransition(m2, m3,
		PacketSizeDistribution{}, PacketSizeDistribution{},
		time.Second)
	if err != ErrMatrixDimensionMismatch {
		t.Fatalf("expected ErrMatrixDimensionMismatch, got %v", err)
	}
}
