package fluid

import (
	"math"
	"runtime"
	"testing"
)

func TestNewPanicsOnInvalidSize(t *testing.T) {
	for _, n := range []int{15, 129} {
		n := n
		t.Run("invalid", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("New(%d) did not panic", n)
				}
			}()
			_ = New(n)
		})
	}
	for _, n := range []int{16, 128} {
		s := New(n)
		if s == nil || s.n != n {
			t.Fatalf("New(%d) = %#v", n, s)
		}
	}
}

func TestStepFiniteAndBounded(t *testing.T) {
	s := New(32)
	for i := 0; i < 180; i++ {
		s.Step(float32(1.0 / 60.0))
	}

	energy := 0.0
	maxSpeed := float32(0)
	totalDye := 0.0
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			if badFloat32(s.u[idx]) || badFloat32(s.v[idx]) || badFloat32(s.r[idx]) || badFloat32(s.g[idx]) || badFloat32(s.b[idx]) {
				t.Fatalf("non-finite field at (%d,%d)", i, j)
			}
			speed2 := s.u[idx]*s.u[idx] + s.v[idx]*s.v[idx]
			energy += float64(speed2)
			speed := float32(math.Sqrt(float64(speed2)))
			if speed > maxSpeed {
				maxSpeed = speed
			}
			totalDye += float64(s.r[idx] + s.g[idx] + s.b[idx])
		}
	}
	if energy <= 1 {
		t.Fatalf("kinetic energy too small: %g", energy)
	}
	if energy >= 2000 {
		t.Fatalf("kinetic energy too large: %g", energy)
	}
	if maxSpeed >= 12 {
		t.Fatalf("max speed too large: %g", maxSpeed)
	}
	if totalDye <= 30 {
		t.Fatalf("total dye too small: %g", totalDye)
	}
	if totalDye >= 8000 {
		t.Fatalf("total dye too large: %g", totalDye)
	}
}

func TestDeterministicReference(t *testing.T) {
	const (
		n     = 40
		steps = 72
		dt    = float32(1.0 / 60.0)
	)

	s1 := New(n)
	s2 := New(n)
	for i := 0; i < steps; i++ {
		s1.Step(dt)
		s2.Step(dt)
	}
	dst1 := make([]byte, n*n*4)
	dst2 := make([]byte, n*n*4)
	s1.RGBA(dst1)
	s2.RGBA(dst2)
	// Quantised renderings tolerate one output level of rounding. Byte-exact
	// equality is not the signal contract; also bound the aggregate error.
	var squaredError float64
	for i := range dst1 {
		d := absDiffByte(dst1[i], dst2[i])
		if d > 1 {
			t.Fatalf("RGBA error at byte %d: %d levels", i, d)
		}
		squaredError += float64(d * d)
	}
	if rmse := math.Sqrt(squaredError / float64(len(dst1))); rmse > 0.5 {
		t.Fatalf("RGBA RMSE %g exceeds half a quantisation level", rmse)
	}

	st := stateStats(s1)
	byteSum, weighted := imageStats(dst1)
	approxEqual(t, "energy", st.energy, 1190.076508, 18.0)
	approxEqual(t, "r sum", st.rSum, 175.810310, 3.0)
	approxEqual(t, "g sum", st.gSum, 238.288824, 3.5)
	approxEqual(t, "b sum", st.bSum, 201.658005, 3.5)
	approxEqual(t, "max speed", st.maxSpeed, 2.088007, 0.05)
	approxEqual(t, "byte sum", byteSum, 67403.2696, 900.0)
	approxEqual(t, "weighted luma", weighted, 56210405.543, 900000.0)
}

func TestRGBAOutputNonTrivialAndOpaque(t *testing.T) {
	s := New(48)
	for i := 0; i < 96; i++ {
		s.Step(float32(1.0 / 60.0))
	}
	dst := make([]byte, s.n*s.n*4)
	s.RGBA(dst)

	colored := 0
	totalVariation := 0
	channelMask := byte(0)
	for i := 0; i < len(dst); i += 4 {
		if dst[i+3] != 0xff {
			t.Fatalf("alpha at pixel %d = %d", i/4, dst[i+3])
		}
		if dst[i] != 0 {
			channelMask |= 1
		}
		if dst[i+1] != 0 {
			channelMask |= 2
		}
		if dst[i+2] != 0 {
			channelMask |= 4
		}
		if dst[i] != 0 || dst[i+1] != 0 || dst[i+2] != 0 {
			colored++
		}
		if i >= 4 {
			totalVariation += absDiffByte(dst[i], dst[i-4])
			totalVariation += absDiffByte(dst[i+1], dst[i-3])
			totalVariation += absDiffByte(dst[i+2], dst[i-2])
		}
	}
	if colored < s.n*s.n/10 {
		t.Fatalf("too few colored pixels: %d", colored)
	}
	if channelMask != 0x7 {
		t.Fatalf("expected all RGB channels to contribute, mask=%03b", channelMask)
	}
	if totalVariation < s.n*s.n {
		t.Fatalf("image variation too small: %d", totalVariation)
	}
}

func TestStepRGBAAllocs(t *testing.T) {
	s := New(32)
	dst := make([]byte, s.n*s.n*4)
	s.Step(float32(1.0 / 60.0))
	s.RGBA(dst)
	allocs := testing.AllocsPerRun(200, func() {
		s.Step(float32(1.0 / 60.0))
		s.RGBA(dst)
	})
	if allocs != 0 {
		t.Fatalf("allocs per Step+RGBA = %g, want 0", allocs)
	}
}

func BenchmarkStepRGBA(b *testing.B) {
	s := New(64)
	dst := make([]byte, s.n*s.n*4)
	for i := 0; i < 24; i++ {
		s.Step(float32(1.0 / 60.0))
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Step(float32(1.0 / 60.0))
		s.RGBA(dst)
	}
	runtime.KeepAlive(dst)
}

type stats struct {
	energy   float64
	rSum     float64
	gSum     float64
	bSum     float64
	maxSpeed float64
}

func stateStats(s *Solver) stats {
	var st stats
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			speed2 := float64(s.u[idx]*s.u[idx] + s.v[idx]*s.v[idx])
			st.energy += speed2
			st.rSum += float64(s.r[idx])
			st.gSum += float64(s.g[idx])
			st.bSum += float64(s.b[idx])
			speed := math.Sqrt(speed2)
			if speed > st.maxSpeed {
				st.maxSpeed = speed
			}
		}
	}
	return st
}

func imageStats(dst []byte) (byteSum, weighted float64) {
	for i := 0; i < len(dst); i += 4 {
		luma := 0.2126*float64(dst[i]) + 0.7152*float64(dst[i+1]) + 0.0722*float64(dst[i+2])
		byteSum += luma
		weighted += luma * float64(1+i/4)
	}
	return byteSum, weighted
}

func approxEqual(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.6f, want %.6f ± %.6f", name, got, want, tol)
	}
}

func badFloat32(v float32) bool {
	return math.IsNaN(float64(v)) || math.IsInf(float64(v), 0)
}

func absDiffByte(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
