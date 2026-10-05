package fluid

import "math"

const (
	minGridSize = 16
	maxGridSize = 128
	linIters    = 18
	maxStepDT   = 0.1
)

// Solver advances a self-driven, incompressible RGB fluid on a square grid.
//
// The implementation follows the classic stable-fluids split step:
// semi-Lagrangian advection with iterative diffusion and projection, while
// reusing fixed buffers allocated during construction.
type Solver struct {
	n      int
	stride int
	size   int

	viscosity float32
	diffusion float32
	velDamp   float32
	dyeDamp   float32
	radius    float32

	u   []float32
	v   []float32
	u0  []float32
	v0  []float32
	p   []float32
	div []float32

	r  []float32
	g  []float32
	b  []float32
	r0 []float32
	g0 []float32
	b0 []float32

	time float32
	step uint64
}

// New constructs an n x n solver.
//
// n must be in the inclusive range [16, 128].
func New(n int) *Solver {
	if n < minGridSize || n > maxGridSize {
		panic("fluid: grid size must be in [16, 128]")
	}
	stride := n + 2
	size := stride * stride
	radius := 0.12 * float32(n)
	if radius < 2.5 {
		radius = 2.5
	}
	return &Solver{
		n:         n,
		stride:    stride,
		size:      size,
		viscosity: 0.0007,
		diffusion: 0.00005,
		velDamp:   0.72,
		dyeDamp:   0.24,
		radius:    radius,
		u:         make([]float32, size),
		v:         make([]float32, size),
		u0:        make([]float32, size),
		v0:        make([]float32, size),
		p:         make([]float32, size),
		div:       make([]float32, size),
		r:         make([]float32, size),
		g:         make([]float32, size),
		b:         make([]float32, size),
		r0:        make([]float32, size),
		g0:        make([]float32, size),
		b0:        make([]float32, size),
	}
}

// Step advances the simulation by dt seconds.
func (s *Solver) Step(dt float32) {
	if dt <= 0 {
		return
	}
	if dt > maxStepDT {
		dt = maxStepDT
	}

	s.step++
	s.addDrive(dt)

	copy(s.u0, s.u)
	s.diffuse(1, s.u, s.u0, s.viscosity, dt)
	copy(s.v0, s.v)
	s.diffuse(2, s.v, s.v0, s.viscosity, dt)
	s.project(s.u, s.v, s.p, s.div)

	copy(s.u0, s.u)
	copy(s.v0, s.v)
	s.advect(1, s.u, s.u0, s.u0, s.v0, dt)
	s.advect(2, s.v, s.v0, s.u0, s.v0, dt)
	s.project(s.u, s.v, s.p, s.div)

	copy(s.r0, s.r)
	s.diffuse(0, s.r, s.r0, s.diffusion, dt)
	copy(s.g0, s.g)
	s.diffuse(0, s.g, s.g0, s.diffusion, dt)
	copy(s.b0, s.b)
	s.diffuse(0, s.b, s.b0, s.diffusion, dt)

	copy(s.r0, s.r)
	copy(s.g0, s.g)
	copy(s.b0, s.b)
	s.advect(0, s.r, s.r0, s.u, s.v, dt)
	s.advect(0, s.g, s.g0, s.u, s.v, dt)
	s.advect(0, s.b, s.b0, s.u, s.v, dt)

	s.applyDecay(dt)
}

// RGBA writes the current dye field to dst as packed, opaque RGBA pixels.
// dst must have length at least n*n*4.
func (s *Solver) RGBA(dst []byte) {
	need := s.n * s.n * 4
	if len(dst) < need {
		panic("fluid: RGBA destination too small")
	}
	k := 0
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			dst[k+0] = tone8(s.r[idx])
			dst[k+1] = tone8(s.g[idx])
			dst[k+2] = tone8(s.b[idx])
			dst[k+3] = 0xff
			k += 4
		}
	}
}

func (s *Solver) idx(i, j int) int {
	return i + j*s.stride
}

func (s *Solver) addDrive(dt float32) {
	s.time += dt
	t := float64(s.time)
	n := float32(s.n)

	leftX := 0.30*n + 0.09*n*float32(math.Sin(0.83*t))
	leftY := 0.20*n + 0.03*n*float32(math.Cos(1.07*t))
	rightX := 0.70*n + 0.08*n*float32(math.Cos(0.71*t+0.8))
	rightY := 0.19*n + 0.04*n*float32(math.Sin(0.97*t+1.1))
	midX := 0.50*n + 0.05*n*float32(math.Sin(0.55*t+2.1))
	midY := 0.26*n + 0.02*n*float32(math.Cos(1.33*t+0.4))

	pulseA := 0.75 + 0.25*float32(math.Sin(1.21*t))
	pulseB := 0.70 + 0.30*float32(math.Cos(0.91*t))
	pulseC := 0.80 + 0.20*float32(math.Sin(1.77*t+0.6))

	s.inject(dt, leftX, leftY, 0.45, 2.40, +1.60, 1.35*pulseA, 0.45, 0.18)
	s.inject(dt, rightX, rightY, -0.50, 2.30, -1.55, 0.20, 0.85*pulseB, 1.30)
	s.inject(dt, midX, midY, 0.00, 1.80, 0.95*float32(math.Sin(0.67*t+0.3)), 0.30, 1.20, 0.55*pulseC)
}

func (s *Solver) inject(dt, cx, cy, jetX, jetY, spin, cr, cg, cb float32) {
	radius := s.radius
	radius2 := radius * radius
	force := dt * (10.5 + 0.06*float32(s.n))
	dye := dt * (3.4 + 0.02*float32(s.n))

	minX := clampInt(int(cx-radius)-1, 1, s.n)
	maxX := clampInt(int(cx+radius)+1, 1, s.n)
	minY := clampInt(int(cy-radius)-1, 1, s.n)
	maxY := clampInt(int(cy+radius)+1, 1, s.n)

	for j := minY; j <= maxY; j++ {
		dy := float32(j) - cy
		row := j * s.stride
		for i := minX; i <= maxX; i++ {
			dx := float32(i) - cx
			r2 := dx*dx + dy*dy
			if r2 >= radius2 {
				continue
			}
			w := 1 - r2/radius2
			w *= w
			idx := row + i
			s.u[idx] += w * force * (jetX - dy*spin/radius)
			s.v[idx] += w * force * (jetY + dx*spin/radius)
			s.r[idx] += w * dye * cr
			s.g[idx] += w * dye * cg
			s.b[idx] += w * dye * cb
		}
	}
}

func (s *Solver) diffuse(boundary int, x, x0 []float32, diff, dt float32) {
	a := dt * diff * float32(s.n*s.n)
	s.linSolve(boundary, x, x0, a, 1+4*a)
}

func (s *Solver) linSolve(boundary int, x, x0 []float32, a, c float32) {
	invC := 1 / c
	for k := 0; k < linIters; k++ {
		for j := 1; j <= s.n; j++ {
			row := j * s.stride
			up := row - s.stride
			down := row + s.stride
			for i := 1; i <= s.n; i++ {
				idx := row + i
				x[idx] = (x0[idx] + a*(x[idx-1]+x[idx+1]+x[up+i]+x[down+i])) * invC
			}
		}
		s.setBoundary(boundary, x)
	}
}

func (s *Solver) project(u, v, p, div []float32) {
	invN := 1 / float32(s.n)
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		up := row - s.stride
		down := row + s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			div[idx] = -0.5 * invN * (u[idx+1] - u[idx-1] + v[down+i] - v[up+i])
			p[idx] = 0
		}
	}
	s.setBoundary(0, div)
	s.setBoundary(0, p)
	s.linSolve(0, p, div, 1, 4)

	scale := 0.5 * float32(s.n)
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		up := row - s.stride
		down := row + s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			u[idx] -= scale * (p[idx+1] - p[idx-1])
			v[idx] -= scale * (p[down+i] - p[up+i])
		}
	}
	s.setBoundary(1, u)
	s.setBoundary(2, v)
}

func (s *Solver) advect(boundary int, d, d0, u, v []float32, dt float32) {
	dt0 := dt * float32(s.n)
	maxPos := float32(s.n) + 0.5
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			x := float32(i) - dt0*u[idx]
			y := float32(j) - dt0*v[idx]
			if x < 0.5 {
				x = 0.5
			} else if x > maxPos {
				x = maxPos
			}
			if y < 0.5 {
				y = 0.5
			} else if y > maxPos {
				y = maxPos
			}
			i0 := int(x)
			i1 := i0 + 1
			j0 := int(y)
			j1 := j0 + 1
			s1 := x - float32(i0)
			s0 := 1 - s1
			t1 := y - float32(j0)
			t0 := 1 - t1
			d[idx] = s0*(t0*d0[s.idx(i0, j0)]+t1*d0[s.idx(i0, j1)]) +
				s1*(t0*d0[s.idx(i1, j0)]+t1*d0[s.idx(i1, j1)])
		}
	}
	s.setBoundary(boundary, d)
}

func (s *Solver) applyDecay(dt float32) {
	velDecay := float32(math.Exp(-float64(s.velDamp * dt)))
	dyeDecay := float32(math.Exp(-float64(s.dyeDamp * dt)))
	for j := 1; j <= s.n; j++ {
		row := j * s.stride
		for i := 1; i <= s.n; i++ {
			idx := row + i
			s.u[idx] *= velDecay
			s.v[idx] *= velDecay
			s.r[idx] *= dyeDecay
			s.g[idx] *= dyeDecay
			s.b[idx] *= dyeDecay
			if s.r[idx] < 0 {
				s.r[idx] = 0
			} else if s.r[idx] > 4 {
				s.r[idx] = 4
			}
			if s.g[idx] < 0 {
				s.g[idx] = 0
			} else if s.g[idx] > 4 {
				s.g[idx] = 4
			}
			if s.b[idx] < 0 {
				s.b[idx] = 0
			} else if s.b[idx] > 4 {
				s.b[idx] = 4
			}
		}
	}
	s.setBoundary(1, s.u)
	s.setBoundary(2, s.v)
	s.setBoundary(0, s.r)
	s.setBoundary(0, s.g)
	s.setBoundary(0, s.b)
}

func (s *Solver) setBoundary(boundary int, x []float32) {
	for i := 1; i <= s.n; i++ {
		if boundary == 1 {
			x[s.idx(0, i)] = -x[s.idx(1, i)]
			x[s.idx(s.n+1, i)] = -x[s.idx(s.n, i)]
		} else {
			x[s.idx(0, i)] = x[s.idx(1, i)]
			x[s.idx(s.n+1, i)] = x[s.idx(s.n, i)]
		}
		if boundary == 2 {
			x[s.idx(i, 0)] = -x[s.idx(i, 1)]
			x[s.idx(i, s.n+1)] = -x[s.idx(i, s.n)]
		} else {
			x[s.idx(i, 0)] = x[s.idx(i, 1)]
			x[s.idx(i, s.n+1)] = x[s.idx(i, s.n)]
		}
	}
	x[s.idx(0, 0)] = 0.5 * (x[s.idx(1, 0)] + x[s.idx(0, 1)])
	x[s.idx(0, s.n+1)] = 0.5 * (x[s.idx(1, s.n+1)] + x[s.idx(0, s.n)])
	x[s.idx(s.n+1, 0)] = 0.5 * (x[s.idx(s.n, 0)] + x[s.idx(s.n+1, 1)])
	x[s.idx(s.n+1, s.n+1)] = 0.5 * (x[s.idx(s.n, s.n+1)] + x[s.idx(s.n+1, s.n)])
}

func tone8(x float32) byte {
	if x <= 0 {
		return 0
	}
	y := 1 - math.Exp(-1.45*float64(x))
	if y <= 0 {
		return 0
	}
	if y >= 1 {
		return 0xff
	}
	return byte(y*255 + 0.5)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
