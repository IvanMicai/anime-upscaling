package dualaudio

import "math"

// Analysis resolution. 16 kHz mono is plenty: what lines the two tracks up is
// music and sound effects, not voice timbre. 10 ms per frame is half a video
// frame before sub-frame interpolation even starts.
const (
	featRate   = 16000
	hopSamples = 160 // 10 ms
	winSamples = 400 // 25 ms
	nfft       = 512
	numBands   = 24
	fps        = float64(featRate) / float64(hopSamples) // 100 frames/s
)

// Features is a band-by-frame matrix, z-scored per band.
type Features struct {
	Bands  [][]float64 // [numBands][frames]
	Frames int
}

// slice returns a view over frames [a, b).
func (f *Features) slice(a, b int) *Features {
	out := &Features{Bands: make([][]float64, len(f.Bands)), Frames: b - a}
	for i := range f.Bands {
		out.Bands[i] = f.Bands[i][a:b]
	}
	return out
}

func melFilterbank() [][]float64 {
	hz2mel := func(f float64) float64 { return 2595 * math.Log10(1+f/700) }
	mel2hz := func(m float64) float64 { return 700 * (math.Pow(10, m/2595) - 1) }
	const fmin, fmax = 80.0, 7000.0
	nbins := nfft/2 + 1
	pts := make([]int, numBands+2)
	lo, hi := hz2mel(fmin), hz2mel(fmax)
	for i := range pts {
		m := lo + (hi-lo)*float64(i)/float64(numBands+1)
		pts[i] = int(math.Floor(float64(nfft+1) * mel2hz(m) / featRate))
	}
	fb := make([][]float64, numBands)
	for i := range fb {
		fb[i] = make([]float64, nbins)
		l, c, r := pts[i], pts[i+1], pts[i+2]
		if c <= l {
			c = l + 1
		}
		if r <= c {
			r = c + 1
		}
		if r > nbins {
			r = nbins
		}
		for k := l; k < c && k < nbins; k++ {
			fb[i][k] = float64(k-l) / float64(c-l)
		}
		for k := c; k < r; k++ {
			fb[i][k] = 1 - float64(k-c)/float64(r-c)
		}
	}
	return fb
}

// ExtractFeatures turns mono 16 kHz PCM into positive spectral flux per mel
// band.
//
// Why flux and not plain log-mel energy: two dubs of the same episode share
// music and sound effects but not voices. Log-mel follows the overall envelope,
// and the loudest thing varying between the tracks is exactly the dialogue.
// Positive flux keeps only what RISES from one frame to the next, which
// sharpens note onsets and effects (identical in both masters) and mutes the
// sustained timbre of speech (not identical). On the episodes this was
// validated against it lifted window confidence from ~8 to 20-140.
func ExtractFeatures(pcm []int16) *Features {
	if len(pcm) < winSamples {
		return &Features{Bands: make([][]float64, numBands)}
	}
	frames := 1 + (len(pcm)-winSamples)/hopSamples
	fb := melFilterbank()
	hann := make([]float64, winSamples)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(winSamples-1))
	}

	logmel := make([][]float64, numBands)
	for b := range logmel {
		logmel[b] = make([]float64, frames)
	}
	re := make([]float64, nfft)
	im := make([]float64, nfft)
	power := make([]float64, nfft/2+1)
	for t := 0; t < frames; t++ {
		off := t * hopSamples
		for i := range re {
			re[i], im[i] = 0, 0
		}
		for i := 0; i < winSamples; i++ {
			re[i] = float64(pcm[off+i]) / 32768 * hann[i]
		}
		fft(re, im, false)
		for k := range power {
			power[k] = re[k]*re[k] + im[k]*im[k]
		}
		for b := 0; b < numBands; b++ {
			var e float64
			for k, w := range fb[b] {
				if w != 0 {
					e += w * power[k]
				}
			}
			logmel[b][t] = math.Log(e + 1e-8)
		}
	}

	out := &Features{Bands: make([][]float64, numBands), Frames: frames - 1}
	for b := 0; b < numBands; b++ {
		flux := make([]float64, frames-1)
		for t := range flux {
			if d := logmel[b][t+1] - logmel[b][t]; d > 0 {
				flux[t] = d
			}
		}
		zscore(flux)
		out.Bands[b] = flux
	}
	return out
}

// zscore normalises in place. It removes level and EQ differences between the
// two masters so correlation measures temporal shape only.
func zscore(x []float64) {
	if len(x) == 0 {
		return
	}
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	var ss float64
	for _, v := range x {
		ss += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(ss / float64(len(x)))
	if sd < 1e-6 {
		sd = 1e-6
	}
	for i := range x {
		x[i] = (x[i] - mean) / sd
	}
}
