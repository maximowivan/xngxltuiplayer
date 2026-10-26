package audio

import "math"

const fftSize = 2048

// fft performs an in-place radix-2 Cooley-Tukey FFT. len(re) must be a power
// of two and equal to len(im).
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}
	if n&(n-1) != 0 {
		return
	}

	// Bit reversal permutation.
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
		angle := -2 * math.Pi / float64(length)
		wRe, wIm := math.Cos(angle), math.Sin(angle)
		for i := 0; i < n; i += length {
			curRe, curIm := 1.0, 0.0
			half := length >> 1
			for j := 0; j < half; j++ {
				uRe, uIm := re[i+j], im[i+j]
				vRe := re[i+j+half]*curRe - im[i+j+half]*curIm
				vIm := re[i+j+half]*curIm + im[i+j+half]*curRe
				re[i+j], im[i+j] = uRe+vRe, uIm+vIm
				re[i+j+half], im[i+j+half] = uRe-vRe, uIm-vIm
				curRe, curIm = curRe*wRe-curIm*wIm, curRe*wIm+curIm*wRe
			}
		}
	}
}

// spectrumFromSamples computes logarithmically spaced magnitude bars from
// 48 kHz mono samples. The result is normalized to 0..1.
func spectrumFromSamples(samples []float64, bars int) []float64 {
	if bars <= 0 {
		return nil
	}
	out := make([]float64, bars)
	n := fftSize
	if len(samples) < n {
		padded := make([]float64, n)
		copy(padded[n-len(samples):], samples)
		samples = padded
	} else if len(samples) > n {
		samples = samples[len(samples)-n:]
	}

	re := make([]float64, n)
	im := make([]float64, n)
	for i := 0; i < n; i++ {
		window := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		re[i] = samples[i] * window
	}
	fft(re, im)

	const fMin, fMax = 45.0, 16000.0
	ratio := fMax / fMin
	nyquistBin := n/2 - 1
	for i := 0; i < bars; i++ {
		f0 := fMin * math.Pow(ratio, float64(i)/float64(bars))
		f1 := fMin * math.Pow(ratio, float64(i+1)/float64(bars))
		b0 := int(f0 * float64(n) / float64(SampleRate))
		b1 := int(f1 * float64(n) / float64(SampleRate))
		if b0 < 1 {
			b0 = 1
		}
		if b1 <= b0 {
			b1 = b0 + 1
		}
		if b1 > nyquistBin {
			b1 = nyquistBin
		}
		peak := 0.0
		for b := b0; b <= b1; b++ {
			mag := math.Hypot(re[b], im[b]) * 4 / float64(n)
			if mag > peak {
				peak = mag
			}
		}
		db := 20 * math.Log10(peak+1e-9)
		v := (db + 58) / 58
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		out[i] = math.Pow(v, 1.35)
	}
	return out
}
