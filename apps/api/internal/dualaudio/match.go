package dualaudio

import (
	"math"
	"sort"
	"sync"
)

// Pairing episodes by number is wrong more often than it looks. Two releases of
// one season routinely disagree on episode ORDER (broadcast order vs production
// order): in the validation set 28 of 82 numbers differed, by anything from -26
// to +6. Pairing by number would have produced dozens of files carrying another
// episode's dialogue. So the pair is decided by the audio, and how well it
// matched is part of the answer.

const (
	fingerprintFPS = 10.0
	// maxShiftSec is how far apart the two releases may start. 150 s covers a
	// missing cold open plus opening theme; the largest seen in validation was
	// under 60 s.
	maxShiftSec = 150.0
)

// Fingerprint is a coarse (10 fps) copy of the features: cheap enough to compare
// every base against every candidate dub.
func Fingerprint(f *Features) *Features {
	k := int(math.Round(fps / fingerprintFPS))
	n := f.Frames / k
	out := &Features{Bands: make([][]float64, len(f.Bands)), Frames: n}
	for b := range f.Bands {
		row := make([]float64, n)
		for i := range row {
			for j := 0; j < k; j++ {
				row[i] += f.Bands[b][i*k+j]
			}
		}
		zscore(row)
		out.Bands[b] = row
	}
	return out
}

// baseSpectrum is the transform of one zero-padded base fingerprint, kept so
// that scoring N dubs against it costs N dub transforms rather than N base
// transforms as well.
type baseSpectrum struct {
	re, im [][]float64
	size   int // transform length
	n      int // padded base length, in frames
}

func newBaseSpectrum(base *Features, maxDubFrames int) *baseSpectrum {
	pad := int(maxShiftSec * fingerprintFPS)
	bs := &baseSpectrum{n: base.Frames + 2*pad}
	bs.size = nextPow2(bs.n + maxDubFrames)
	for _, band := range base.Bands {
		re := make([]float64, bs.size)
		im := make([]float64, bs.size)
		copy(re[pad:], band)
		fft(re, im, false)
		bs.re, bs.im = append(bs.re, re), append(bs.im, im)
	}
	return bs
}

// confidence scores one dub: peak of the summed per-band correlation against
// the robust spread (MAD) of the whole curve.
func (bs *baseSpectrum) confidence(dub *Features) float64 {
	m := dub.Frames
	if m < 10 || bs.n < m || bs.n+m > bs.size {
		return 0
	}
	scores := make([]float64, bs.n-m+1)
	re := make([]float64, bs.size)
	im := make([]float64, bs.size)
	for b, band := range dub.Bands {
		for i := range re {
			re[i], im[i] = 0, 0
		}
		for i := 0; i < m; i++ {
			re[i] = band[m-1-i]
		}
		fft(re, im, false)
		for i := range re {
			r := bs.re[b][i]*re[i] - bs.im[b][i]*im[i]
			re[i], im[i] = r, bs.re[b][i]*im[i]+bs.im[b][i]*re[i]
		}
		fft(re, im, true)
		for k := range scores {
			scores[k] += re[m-1+k]
		}
	}
	peak := scores[0]
	for _, v := range scores {
		if v > peak {
			peak = v
		}
	}
	med := median(scores)
	dev := make([]float64, len(scores))
	for i, v := range scores {
		dev[i] = math.Abs(v - med)
	}
	return (peak - med) / (1.4826*median(dev) + 1e-9)
}

// Compare returns the confidence of the best alignment between two
// fingerprints, allowing the dub to start up to maxShiftSec before or after the
// base.
func Compare(base, dub *Features) float64 {
	if base.Frames < 10 || dub.Frames < 10 {
		return 0
	}
	return newBaseSpectrum(base, dub.Frames).confidence(dub)
}

// Candidate is one dub scored against a base.
type Candidate struct {
	Key        string
	Confidence float64
}

// Pair is the decision for one base episode. Dub is empty when nothing matched
// clearly enough — a refusal, not a failure: an ambiguous pair that gets built
// anyway is a file with the wrong episode's dialogue.
type Pair struct {
	Base       string  `json:"base"`
	Dub        string  `json:"dub,omitempty"`
	Confidence float64 `json:"confidence"`
	Margin     float64 `json:"margin"`
	Note       string  `json:"note,omitempty"`
}

// BestMatch picks the dub for one base. minMargin is the required ratio between
// the best and second-best candidate; a genuine pair typically clears 3x, while
// an episode with no counterpart shows several equally poor candidates.
func BestMatch(baseKey string, base *Features, dubs map[string]*Features, minMargin float64) Pair {
	maxDub := 0
	for _, d := range dubs {
		if d.Frames > maxDub {
			maxDub = d.Frames
		}
	}
	cands := make([]Candidate, 0, len(dubs))
	if base.Frames >= 10 && maxDub >= 10 {
		bs := newBaseSpectrum(base, maxDub)
		for key, d := range dubs {
			cands = append(cands, Candidate{key, bs.confidence(d)})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Confidence != cands[j].Confidence {
			return cands[i].Confidence > cands[j].Confidence
		}
		return cands[i].Key < cands[j].Key
	})
	p := Pair{Base: baseKey}
	if len(cands) == 0 || cands[0].Confidence <= 0 {
		p.Note = "no candidate scored"
		return p
	}
	p.Confidence = cands[0].Confidence
	second := 0.0
	if len(cands) > 1 {
		second = cands[1].Confidence
	}
	p.Margin = cands[0].Confidence / math.Max(second, 1e-6)
	if p.Margin < minMargin {
		p.Note = "ambiguous: best candidate " + cands[0].Key + " does not clearly beat " + cands[1].Key
		return p
	}
	p.Dub = cands[0].Key
	return p
}

// FlagContested marks pairs whose dub was claimed by more than one base: at
// least one of those claims is wrong.
func FlagContested(pairs []Pair) {
	claims := map[string]int{}
	for _, p := range pairs {
		if p.Dub != "" {
			claims[p.Dub]++
		}
	}
	for i := range pairs {
		if pairs[i].Dub != "" && claims[pairs[i].Dub] > 1 {
			pairs[i].Note = "dub claimed by more than one base episode"
		}
	}
}

// mutualMargin is the bar for a pair that is also a MUTUAL best match (the dub's
// own best base is this base). Measured on a full season: a 1.6x margin alone
// accepted 75 of 82 pairs, all correct, and refused 7 whose top candidate was
// right but whose runner-up — an episode scored with the same music library —
// came close. Adding "or mutual best at 1.15x" accepted 82 of 82, still with no
// wrong pair. Mutuality is what keeps an episode with NO counterpart refused:
// its least-bad dub already belongs, more strongly, to another base.
const mutualMargin = 1.15

// MatchAll pairs every base with its dub, comparing each base against EVERY
// dub. Episode numbers are exactly what cannot be trusted, so they are not used
// to narrow the candidates — a windowed search (+-12 episodes) was tried first
// and missed two pairs that sat 26 numbers apart.
//
// All-against-all is quadratic, so each base's spectrum is computed once rather
// than once per pair, and bases are spread over workers.
func MatchAll(bases, dubs map[string]*Features, minMargin float64, workers int) []Pair {
	baseKeys := sortedKeys(bases)
	dubKeys := sortedKeys(dubs)
	maxDub := 0
	for _, d := range dubs {
		if d.Frames > maxDub {
			maxDub = d.Frames
		}
	}
	if workers < 1 {
		workers = 1
	}
	conf := make([][]float64, len(baseKeys))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				row := make([]float64, len(dubKeys))
				if b := bases[baseKeys[i]]; b.Frames >= 10 && maxDub >= 10 {
					bs := newBaseSpectrum(b, maxDub)
					for j, dk := range dubKeys {
						row[j] = bs.confidence(dubs[dk])
					}
				}
				conf[i] = row
			}
		}()
	}
	for i := range baseKeys {
		next <- i
	}
	close(next)
	wg.Wait()
	return decidePairs(baseKeys, dubKeys, conf, minMargin)
}

func sortedKeys(m map[string]*Features) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// decidePairs turns a base-by-dub confidence matrix into pairing decisions.
func decidePairs(baseKeys, dubKeys []string, conf [][]float64, minMargin float64) []Pair {
	// Best base for each dub, for the mutuality check.
	dubBest := make([]int, len(dubKeys))
	for j := range dubKeys {
		for i := range baseKeys {
			if conf[i][j] > conf[dubBest[j]][j] {
				dubBest[j] = i
			}
		}
	}
	pairs := make([]Pair, len(baseKeys))
	for i, bk := range baseKeys {
		p := Pair{Base: bk}
		best, second := -1, -1
		for j := range dubKeys {
			switch {
			case best < 0 || conf[i][j] > conf[i][best]:
				best, second = j, best
			case second < 0 || conf[i][j] > conf[i][second]:
				second = j
			}
		}
		if best < 0 || conf[i][best] <= 0 {
			p.Note = "no candidate scored"
			pairs[i] = p
			continue
		}
		p.Confidence = conf[i][best]
		runnerUp := 0.0
		if second >= 0 {
			runnerUp = conf[i][second]
		}
		p.Margin = p.Confidence / math.Max(runnerUp, 1e-6)
		mutual := dubBest[best] == i
		switch {
		case p.Margin >= minMargin, mutual && p.Margin >= mutualMargin:
			p.Dub = dubKeys[best]
		case second >= 0:
			p.Note = "ambiguous: best candidate " + dubKeys[best] + " does not clearly beat " + dubKeys[second]
		default:
			p.Note = "ambiguous: single weak candidate " + dubKeys[best]
		}
		pairs[i] = p
	}
	FlagContested(pairs)
	return pairs
}
