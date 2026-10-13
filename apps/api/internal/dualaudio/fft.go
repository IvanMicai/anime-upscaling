package dualaudio

import "math"

// Real-input FFT, written here on purpose: the API module has no third-party
// dependencies (see go.mod) and alignment only needs forward/inverse transforms
// of real signals at power-of-two lengths. Pulling in a DSP library for that
// would be the largest dependency in the repo.

// nextPow2 returns the smallest power of two >= n.
func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// fft computes an in-place radix-2 decimation-in-time FFT. len(re) must be a
// power of two; im is the imaginary part and is modified alongside re.
func fft(re, im []float64, inverse bool) {
	n := len(re)
	if n <= 1 {
		return
	}
	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := 2 * math.Pi / float64(length)
		if !inverse {
			ang = -ang
		}
		wRe, wIm := math.Cos(ang), math.Sin(ang)
		for i := 0; i < n; i += length {
			curRe, curIm := 1.0, 0.0
			for j := 0; j < length/2; j++ {
				uRe, uIm := re[i+j], im[i+j]
				vRe := re[i+j+length/2]*curRe - im[i+j+length/2]*curIm
				vIm := re[i+j+length/2]*curIm + im[i+j+length/2]*curRe
				re[i+j], im[i+j] = uRe+vRe, uIm+vIm
				re[i+j+length/2], im[i+j+length/2] = uRe-vRe, uIm-vIm
				curRe, curIm = curRe*wRe-curIm*wIm, curRe*wIm+curIm*wRe
			}
		}
	}
	if inverse {
		inv := 1 / float64(n)
		for i := range re {
			re[i] *= inv
			im[i] *= inv
		}
	}
}

// crossCorrelate returns c where c[k] = sum_t ref[k+t]*win[t], for k in
// [0, len(ref)-len(win)]. Computed as a circular convolution of ref with the
// reversed window, which is why the transform length has to clear len(ref)+
// len(win) — otherwise the tail wraps onto the head and invents a peak.
func crossCorrelate(ref, win []float64) []float64 {
	n, m := len(ref), len(win)
	if n < m || m == 0 {
		return nil
	}
	size := nextPow2(n + m)
	aRe := make([]float64, size)
	aIm := make([]float64, size)
	bRe := make([]float64, size)
	bIm := make([]float64, size)
	copy(aRe, ref)
	for i := 0; i < m; i++ {
		bRe[i] = win[m-1-i]
	}
	fft(aRe, aIm, false)
	fft(bRe, bIm, false)
	for i := range aRe {
		r := aRe[i]*bRe[i] - aIm[i]*bIm[i]
		im := aRe[i]*bIm[i] + aIm[i]*bRe[i]
		aRe[i], aIm[i] = r, im
	}
	fft(aRe, aIm, true)
	out := make([]float64, n-m+1)
	copy(out, aRe[m-1:m-1+len(out)])
	return out
}
