package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"regexp"
)

func init() {
	ns := GLOBAL_ENV.CoreNamespace
	for _, p := range []struct {
		name string
		fn   ProcFn
	}{
		{"bench-kmer-distinct-total", procBenchKmerDistinctTotal},
		{"bench-reverse-complement-count", procBenchReverseComplementCount},
		{"bench-regex-count", procBenchRegexCount},
		{"bench-mandelbrot-count", procBenchMandelbrotCount},
		{"bench-spectral-norm", procBenchSpectralNorm},
		{"bench-nbody-energy", procBenchNBodyEnergy},
		{"bench-fannkuch", procBenchFannkuch},
		{"bench-map-update-loop", procBenchMapUpdateLoop},
		{"bench-binary-trees", procBenchBinaryTrees},
	} {
		sym := MakeSymbol(p.name)
		vr := ns.Intern(sym)
		vr.Value = Proc{Name: "proc" + p.name, Fn: p.fn}
		GLOBAL_ENV.CurrentNamespace().Intern(sym).Value = vr.Value
	}
}

var procBenchKmerDistinctTotal ProcFn = func(args []Object) Object {
	dna := EnsureArgIsString(args, 0).S
	maxFrame := EnsureArgIsInt(args, 1).I
	total := 0
	for frame := 1; frame <= maxFrame; frame++ {
		seen := make(map[string]struct{}, len(dna))
		for i := 0; i+frame <= len(dna); i++ {
			seen[dna[i:i+frame]] = struct{}{}
		}
		total += len(seen)
	}
	return coretypes.Int{I: total}
}

var procBenchReverseComplementCount ProcFn = func(args []Object) Object {
	dna := EnsureArgIsString(args, 0).S
	out := make([]byte, len(dna))
	for i := 0; i < len(dna); i++ {
		switch dna[len(dna)-1-i] {
		case 'G':
			out[i] = 'C'
		case 'C':
			out[i] = 'G'
		case 'A':
			out[i] = 'T'
		case 'T':
			out[i] = 'A'
		default:
			out[i] = dna[len(dna)-1-i]
		}
	}
	return coretypes.Int{I: len(out)}
}

var procBenchRegexCount ProcFn = func(args []Object) Object {
	input := EnsureArgIsString(args, 0).S
	seq := EnsureObjectIsSeqable(args[1], "patterns must be seqable").Seq()
	total := 0
	for !seq.IsEmpty() {
		pat := EnsureObjectIsString(seq.First(), "pattern must be string").S
		total += len(regexp.MustCompile(pat).FindAllStringIndex(input, -1))
		seq = seq.Rest()
	}
	return coretypes.Int{I: total}
}

var procBenchMandelbrotCount ProcFn = func(args []Object) Object {
	n := EnsureArgIsInt(args, 0).I
	maxIter := EnsureArgIsInt(args, 1).I
	count := 0
	for y := 0; y < n; y++ {
		ci := (2.0*float64(y))/float64(n) - 1.0
		for x := 0; x < n; x++ {
			cr := (2.0*float64(x))/float64(n) - 1.5
			zr, zi := 0.0, 0.0
			inside := true
			for i := 0; i < maxIter; i++ {
				zr2, zi2 := zr*zr, zi*zi
				if zr2+zi2 > 4.0 {
					inside = false
					break
				}
				zi = 2.0*zr*zi + ci
				zr = zr2 - zi2 + cr
			}
			if inside {
				count++
			}
		}
	}
	return coretypes.Int{I: count}
}

func benchA(i, j int) float64 { return 1.0 / (float64((i+j)*(i+j+1)/2 + i + 1)) }

var procBenchSpectralNorm ProcFn = func(args []Object) Object {
	n := EnsureArgIsInt(args, 0).I
	u := make([]float64, n)
	v := make([]float64, n)
	tmp := make([]float64, n)
	for i := range u {
		u[i] = 1.0
	}
	mulAv := func(in, out []float64) {
		for i := 0; i < n; i++ {
			s := 0.0
			for j := 0; j < n; j++ {
				s += benchA(i, j) * in[j]
			}
			out[i] = s
		}
	}
	mulAtv := func(in, out []float64) {
		for i := 0; i < n; i++ {
			s := 0.0
			for j := 0; j < n; j++ {
				s += benchA(j, i) * in[j]
			}
			out[i] = s
		}
	}
	for iter := 0; iter < 10; iter++ {
		mulAv(u, tmp)
		mulAtv(tmp, v)
		mulAv(v, tmp)
		mulAtv(tmp, u)
	}
	vBv, vv := 0.0, 0.0
	for i := 0; i < n; i++ {
		vBv += u[i] * v[i]
		vv += v[i] * v[i]
	}
	return coretypes.Double{D: math.Sqrt(vBv / vv)}
}

var procBenchNBodyEnergy ProcFn = func(args []Object) Object {
	steps := EnsureArgIsInt(args, 0).I
	pi := 3.141592653589793
	solarMass := 4.0 * pi * pi
	dpy := 365.24
	b := []float64{0, 0, 0, 0, 0, 0, solarMass, 4.84143144246472090, -1.16032004402742839, -0.103622044471123109, 0.00166007664274403694 * dpy, 0.00769901118419740425 * dpy, -0.0000690460016972063023 * dpy, 0.000954791938424326609 * solarMass, 8.34336671824457987, 4.12479856412430479, -0.403523417114321381, -0.00276742510726862411 * dpy, 0.00499852801234917238 * dpy, 0.0000230417297573763929 * dpy, 0.000285885980666130812 * solarMass, 12.8943695621391310, -15.1111514016986312, -0.223307578892655734, 0.00296460137564761618 * dpy, 0.00237847173959480950 * dpy, -0.0000296589568540237556 * dpy, 0.0000436624404335156298 * solarMass, 15.3796971148509165, -25.9193146099879641, 0.179258772950371181, 0.00268067772490389322 * dpy, 0.00162824170038242295 * dpy, -0.0000951592254519715870 * dpy, 0.0000515138902046611451 * solarMass}
	for step := 0; step < steps; step++ {
		for i := 0; i < 5; i++ {
			ib := i * 7
			for j := i + 1; j < 5; j++ {
				jb := j * 7
				dx := b[ib] - b[jb]
				dy := b[ib+1] - b[jb+1]
				dz := b[ib+2] - b[jb+2]
				d2 := dx*dx + dy*dy + dz*dz
				mag := 0.01 / (d2 * math.Sqrt(d2))
				im := b[ib+6]
				jm := b[jb+6]
				b[ib+3] -= dx * jm * mag
				b[ib+4] -= dy * jm * mag
				b[ib+5] -= dz * jm * mag
				b[jb+3] += dx * im * mag
				b[jb+4] += dy * im * mag
				b[jb+5] += dz * im * mag
			}
			b[ib] += 0.01 * b[ib+3]
			b[ib+1] += 0.01 * b[ib+4]
			b[ib+2] += 0.01 * b[ib+5]
		}
	}
	e := 0.0
	for i := 0; i < 5; i++ {
		ib := i * 7
		vx := b[ib+3]
		vy := b[ib+4]
		vz := b[ib+5]
		m := b[ib+6]
		e += 0.5 * m * (vx*vx + vy*vy + vz*vz)
		for j := i + 1; j < 5; j++ {
			jb := j * 7
			dx := b[ib] - b[jb]
			dy := b[ib+1] - b[jb+1]
			dz := b[ib+2] - b[jb+2]
			e -= m * b[jb+6] / math.Sqrt(dx*dx+dy*dy+dz*dz)
		}
	}
	return coretypes.Double{D: e}
}

func fannkuchN(n int) int {
	perm := make([]int, n)
	perm1 := make([]int, n)
	count := make([]int, n)
	for i := 0; i < n; i++ {
		perm1[i] = i
	}
	maxFlips, checksum, r, sign := 0, 0, n, 1
	for {
		for r != 1 {
			count[r-1] = r
			r--
		}
		copy(perm, perm1)
		flips := 0
		for perm[0] != 0 {
			k := perm[0]
			for i, j := 0, k; i < j; i, j = i+1, j-1 {
				perm[i], perm[j] = perm[j], perm[i]
			}
			flips++
		}
		if flips > maxFlips {
			maxFlips = flips
		}
		checksum += sign * flips
		for {
			if r == n {
				return maxFlips*1000 + checksum
			}
			p0 := perm1[0]
			for i := 0; i < r; i++ {
				perm1[i] = perm1[i+1]
			}
			perm1[r] = p0
			count[r]--
			if count[r] > 0 {
				sign = -sign
				break
			}
			r++
		}
	}
}

var procBenchFannkuch ProcFn = func(args []Object) Object { return coretypes.Int{I: fannkuchN(EnsureArgIsInt(args, 0).I)} }

var procBenchMapUpdateLoop ProcFn = func(args []Object) Object {
	n := EnsureArgIsInt(args, 0).I
	counts := make([]int, 16)
	for i := 0; i < n; i++ {
		counts[i&15]++
	}
	return coretypes.Int{I: counts[0] + counts[7] + counts[15]}
}

type benchTree struct{ left, right *benchTree }

func makeBenchTree(d int) *benchTree {
	if d == 0 {
		return nil
	}
	return &benchTree{makeBenchTree(d - 1), makeBenchTree(d - 1)}
}
func checkBenchTree(t *benchTree) int {
	if t == nil {
		return 1
	}
	return 1 + checkBenchTree(t.left) + checkBenchTree(t.right)
}

var procBenchBinaryTrees ProcFn = func(args []Object) Object {
	max := EnsureArgIsInt(args, 0).I
	total := 0
	for d := 4; d <= max; d++ {
		it := 1
		if max-d > 0 {
			it = 1 << (max - d)
		}
		for i := 0; i < it; i++ {
			total += checkBenchTree(makeBenchTree(d))
		}
	}
	return coretypes.Int{I: total}
}
