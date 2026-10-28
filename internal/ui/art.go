package ui

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// renderArt turns an image into truecolor half-block cells. width and height
// are measured in terminal cells; height*2 pixel rows are used. When the
// terminal or the selected glyph mode cannot render half blocks, a small ASCII
// note is used instead so the pane keeps its shape.
func renderArt(img image.Image, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	if img == nil || !currentGlyphs.ArtHalfBlock || activeColorProfile == ColorNone {
		return renderASCIIArt(width, height)
	}
	pixelW := width
	pixelH := height * 2
	scaled := image.NewRGBA(image.Rect(0, 0, pixelW, pixelH))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Src, nil)

	lines := make([]string, 0, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			top := scaled.RGBAAt(x, y*2)
			bottom := scaled.RGBAAt(x, y*2+1)
			tr, tg, tb := blendOnDark(top)
			br, bg, bb := blendOnDark(bottom)
			fg := colorSequence(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", tr, tg, tb)), false)
			bgc := colorSequence(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", br, bg, bb)), true)
			b.WriteString(fg)
			b.WriteString(bgc)
			b.WriteRune('▀')
		}
		b.WriteString("\x1b[0m")
		lines = append(lines, b.String())
	}
	return lines
}

// renderASCIIArt draws a compact note with printable characters. It is used on
// classic consoles, in ASCII mode and whenever colour is disabled.
func renderASCIIArt(width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	note := []string{
		"  ##   ",
		"  ## # ",
		"  ## # ",
		"  ##   ",
		"####   ",
		"###    ",
		" #     ",
	}
	scale := 1
	if s := minInt(width/len(note[0]), height/len(note)); s > 1 {
		scale = s
	}
	artW := len(note[0]) * scale
	artH := len(note) * scale
	top := maxInt(0, (height-artH)/2)
	left := maxInt(0, (width-artW)/2)
	lines := make([]string, height)
	for y := 0; y < height; y++ {
		if y < top || y >= top+artH {
			lines[y] = fitLine("", width)
			continue
		}
		src := note[(y-top)/scale]
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", left))
		for _, ch := range src {
			for i := 0; i < scale; i++ {
				if ch == '#' {
					b.WriteByte('#')
				} else {
					b.WriteByte(' ')
				}
			}
		}
		lines[y] = fitLine(accent2Style.Render(b.String()), width)
	}
	return lines
}

func blendOnDark(c color.RGBA) (int, int, int) {
	a := float64(c.A) / 255
	r := float64(c.R)*a + artBgColor[0]*(1-a)
	g := float64(c.G)*a + artBgColor[1]*(1-a)
	b := float64(c.B)*a + artBgColor[2]*(1-a)
	return int(clampByte(r)), int(clampByte(g)), int(clampByte(b))
}

// Palette-derived colors used by placeholder artwork and the spectrum. They
// are refreshed by applyTheme.
var (
	artBgColor     = [3]float64{16, 16, 24}
	artTopColor    = [3]float64{58, 46, 92}
	artBottomColor = [3]float64{20, 17, 31}
	artGlowSign    = 1.0
	specLowColor   = [3]float64{166, 227, 161}
	specMidColor   = [3]float64{249, 226, 175}
	specHighColor  = [3]float64{243, 139, 168}
)

// placeholderImage creates a dark gradient with a simple quaver silhouette.
func placeholderImage(pixelW, pixelH int) image.Image {
	if pixelW <= 0 || pixelH <= 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, pixelW, pixelH))
	top := artTopColor
	bottom := artBottomColor
	for y := 0; y < pixelH; y++ {
		t := float64(y) / math.Max(1, float64(pixelH-1))
		baseR := top[0] + (bottom[0]-top[0])*t
		baseG := top[1] + (bottom[1]-top[1])*t
		baseB := top[2] + (bottom[2]-top[2])*t
		for x := 0; x < pixelW; x++ {
			dx := (float64(x) - float64(pixelW)/2) / math.Max(1, float64(pixelW))
			dy := (float64(y) - float64(pixelH)/2) / math.Max(1, float64(pixelH))
			glow := math.Max(0, 1-math.Sqrt(dx*dx+dy*dy)*2.4) * artGlowSign
			r := baseR + glow*34
			g := baseG + glow*22
			b := baseB + glow*46
			img.SetRGBA(x, y, color.RGBA{clampByte(r), clampByte(g), clampByte(b), 255})
		}
	}

	note := []string{
		"....####..",
		"....####..",
		"....##.##.",
		"....##....",
		"....##....",
		"....##....",
		"....##....",
		"....##....",
		"..####....",
		".#####....",
		".#####....",
		"..###.....",
	}
	scale := maxInt(1, minInt(pixelW, pixelH)*45/100/len(note))
	noteW := len(note[0]) * scale
	noteH := len(note) * scale
	offX := (pixelW - noteW) / 2
	offY := (pixelH - noteH) / 2
	for y, row := range note {
		for x, ch := range row {
			if ch != '#' {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px, py := offX+x*scale+sx, offY+y*scale+sy
					if px >= 0 && px < pixelW && py >= 0 && py < pixelH {
						noteColor := color.RGBA{232, 221, 245, 210}
						if artGlowSign < 0 {
							noteColor = color.RGBA{40, 38, 50, 210}
						}
						img.SetRGBA(px, py, noteColor)
					}
				}
			}
		}
	}
	return img
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// renderSpectrum draws a smooth bar visualizer with one column gap per bar.
func renderSpectrum(levels []float64, width, rows int) []string {
	if width <= 0 || rows <= 0 {
		return nil
	}
	slots := maxInt(1, width/2)
	bars := minInt(len(levels), slots)
	if bars == 0 {
		bars = 1
	}

	silent := true
	for _, v := range levels {
		if v > 0.005 {
			silent = false
			break
		}
	}
	if silent {
		lines := make([]string, rows)
		for i := 0; i < rows-1; i++ {
			lines[i] = strings.Repeat(" ", width)
		}
		baseline := strings.Repeat(currentGlyphs.SpectrumBase, slots)
		lines[rows-1] = fitLine(dimStyle.Render(truncateText(baseline, width)), width)
		return lines
	}

	lines := make([]string, rows)
	for row := 0; row < rows; row++ {
		cellRow := rows - 1 - row
		var b strings.Builder
		for i := 0; i < bars; i++ {
			level := levels[i]
			if level < 0 {
				level = 0
			}
			if level > 1 {
				level = 1
			}
			filled := level*float64(rows) - float64(cellRow)
			if filled <= 0 {
				b.WriteString(" ")
			} else {
				glyph := '#'
				if currentGlyphs.SpectrumBlocks && len(currentGlyphs.Spectrum) > 0 {
					idx := int(math.Ceil(filled * float64(len(currentGlyphs.Spectrum))))
					if idx < 1 {
						idx = 1
					}
					if idx > len(currentGlyphs.Spectrum) {
						idx = len(currentGlyphs.Spectrum)
					}
					glyph = currentGlyphs.Spectrum[idx-1]
				}
				r, g, bl := spectrumRGB(level)
				seq := colorSequence(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", r, g, bl)), false)
				if seq != "" {
					b.WriteString(seq)
				}
				b.WriteRune(glyph)
				if seq != "" {
					b.WriteString("\x1b[0m")
				}
			}
			b.WriteString(" ")
		}
		lines[row] = fitLine(b.String(), width)
	}
	return lines
}

func spectrumRGB(level float64) (int, int, int) {
	type rgb struct{ r, g, b float64 }
	from := rgb{specLowColor[0], specLowColor[1], specLowColor[2]}
	to := rgb{specMidColor[0], specMidColor[1], specMidColor[2]}
	t := 0.0
	if level < 0.5 {
		t = level / 0.5
	} else {
		from = rgb{specMidColor[0], specMidColor[1], specMidColor[2]}
		to = rgb{specHighColor[0], specHighColor[1], specHighColor[2]}
		t = (level - 0.5) / 0.5
	}
	return int(from.r + (to.r-from.r)*t),
		int(from.g + (to.g-from.g)*t),
		int(from.b + (to.b-from.b)*t)
}
