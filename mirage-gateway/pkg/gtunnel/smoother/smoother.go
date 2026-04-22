package smoother

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// MarkovMatrix is an N×N probability transition matrix.
type MarkovMatrix [][]float64

// PacketSizeDistribution holds packet size distribution parameters.
type PacketSizeDistribution struct {
	Mean   float64
	StdDev float64
}

// TimelineCollector is a local interface to avoid circular imports.
type TimelineCollector interface {
	OnModeTransition(reason string) error
}

// DefaultTransitionDuration is the default transition duration.
const DefaultTransitionDuration = 3000 * time.Millisecond

var ErrMatrixDimensionMismatch = errors.New("matrix dimension mismatch")

// TransitionSmoother controls smooth traffic transitions during defense state changes.
type TransitionSmoother struct {
	oldMatrix     MarkovMatrix
	newMatrix     MarkovMatrix
	oldDist       PacketSizeDistribution
	newDist       PacketSizeDistribution
	startTime     time.Time
	duration      time.Duration
	transitioning atomic.Bool
	mu            sync.Mutex
	timeline      TimelineCollector
}

// NewTransitionSmoother creates a new transition smoother.
func NewTransitionSmoother(timeline TimelineCollector) *TransitionSmoother {
	return &TransitionSmoother{
		timeline: timeline,
	}
}

// BeginTransition starts a smooth transition between two Markov matrices.
// If duration is 0, uses DefaultTransitionDuration (3000ms).
// If a transition is in progress, the current interpolated state becomes the new start.
func (s *TransitionSmoother) BeginTransition(
	oldMatrix, newMatrix MarkovMatrix,
	oldDist, newDist PacketSizeDistribution,
	duration time.Duration,
) error {
	if duration <= 0 {
		duration = DefaultTransitionDuration
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// If currently transitioning, use current interpolated state as new start
	var effectiveOld MarkovMatrix
	if s.transitioning.Load() {
		effectiveOld = s.currentMatrixLocked()
		s.oldDist = s.currentDistributionLocked()
	} else {
		effectiveOld = oldMatrix
		s.oldDist = oldDist
	}

	// Validate matrix dimensions
	if len(effectiveOld) != len(newMatrix) {
		return ErrMatrixDimensionMismatch
	}
	for i := range effectiveOld {
		if len(effectiveOld[i]) != len(newMatrix[i]) {
			return ErrMatrixDimensionMismatch
		}
	}

	s.oldMatrix = effectiveOld

	s.newMatrix = newMatrix
	s.newDist = newDist
	s.startTime = time.Now()
	s.duration = duration
	s.transitioning.Store(true)

	return nil
}

// Alpha returns the current interpolation coefficient α(t) = elapsed / duration, clamped to [0, 1].
func (s *TransitionSmoother) Alpha() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alphaLocked()
}

func (s *TransitionSmoother) alphaLocked() float64 {
	if !s.transitioning.Load() {
		return 1.0
	}
	elapsed := time.Since(s.startTime)
	alpha := float64(elapsed) / float64(s.duration)
	if alpha >= 1.0 {
		s.transitioning.Store(false)
		return 1.0
	}
	return alpha
}

// CurrentMatrix returns the interpolated matrix at the current time.
// M(t) = (1 - α) * M_old + α * M_new
func (s *TransitionSmoother) CurrentMatrix() MarkovMatrix {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentMatrixLocked()
}

func (s *TransitionSmoother) currentMatrixLocked() MarkovMatrix {
	alpha := s.alphaLocked()
	return interpolateMatrix(s.oldMatrix, s.newMatrix, alpha)
}

// CurrentDistribution returns the interpolated distribution at the current time.
func (s *TransitionSmoother) CurrentDistribution() PacketSizeDistribution {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentDistributionLocked()
}

func (s *TransitionSmoother) currentDistributionLocked() PacketSizeDistribution {
	alpha := s.alphaLocked()
	return PacketSizeDistribution{
		Mean:   (1-alpha)*s.oldDist.Mean + alpha*s.newDist.Mean,
		StdDev: (1-alpha)*s.oldDist.StdDev + alpha*s.newDist.StdDev,
	}
}

// IsTransitioning returns whether a transition is in progress.
func (s *TransitionSmoother) IsTransitioning() bool {
	return s.transitioning.Load()
}

// InterpolateMatrixAt computes the interpolated matrix for a given alpha (for testing).
func InterpolateMatrixAt(old, new MarkovMatrix, alpha float64) MarkovMatrix {
	return interpolateMatrix(old, new, alpha)
}

// InterpolateDistAt computes the interpolated distribution for a given alpha (for testing).
func InterpolateDistAt(old, new PacketSizeDistribution, alpha float64) PacketSizeDistribution {
	return PacketSizeDistribution{
		Mean:   (1-alpha)*old.Mean + alpha*new.Mean,
		StdDev: (1-alpha)*old.StdDev + alpha*new.StdDev,
	}
}

func interpolateMatrix(old, new MarkovMatrix, alpha float64) MarkovMatrix {
	if len(old) == 0 {
		return new
	}
	n := len(old)
	result := make(MarkovMatrix, n)
	for i := 0; i < n; i++ {
		result[i] = make([]float64, len(old[i]))
		for j := 0; j < len(old[i]); j++ {
			result[i][j] = (1-alpha)*old[i][j] + alpha*new[i][j]
		}
	}
	return result
}

// almostEqual checks float64 equality within tolerance.
func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// Exported for testing
var _ = almostEqual
