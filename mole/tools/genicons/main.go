// Command genicons renders the workflow icon (a mole) as a PNG.
//
//	go run ./tools/genicons workflow
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const size = 256

// rrect is a rounded rectangle in unit coordinates; r = half the side
// gives a circle.
type rrect struct{ x0, y0, x1, y1, r float64 }

func (s rrect) inside(x, y float64) bool {
	if x < s.x0 || x > s.x1 || y < s.y0 || y > s.y1 {
		return false
	}
	cx := math.Max(s.x0+s.r, math.Min(x, s.x1-s.r))
	cy := math.Max(s.y0+s.r, math.Min(y, s.y1-s.r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= s.r*s.r
}

func circle(cx, cy, r float64) rrect { return rrect{cx - r, cy - r, cx + r, cy + r, r} }

type layer struct {
	s rrect
	c color.NRGBA
}

func render(layers []layer, path string) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / size
					y := (float64(py) + (float64(sy)+0.5)/ss) / size
					var cr, cg, cb, ca float64
					for _, l := range layers {
						if l.s.inside(x, y) {
							la := float64(l.c.A) / 255
							cr = cr*(1-la) + float64(l.c.R)*la
							cg = cg*(1-la) + float64(l.c.G)*la
							cb = cb*(1-la) + float64(l.c.B)*la
							ca = ca*(1-la) + la
						}
					}
					r, g, b, a = r+cr, g+cg, b+cb, a+ca
				}
			}
			n := float64(ss * ss)
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{
					R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8(255 * a / n),
				})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func main() {
	dir := "workflow"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	layers := []layer{
		{rrect{0.04, 0.04, 0.96, 0.96, 0.2}, hex(0x1e1e2e)}, // tile
		{circle(0.29, 0.32, 0.09), hex(0x6d5545)},           // ears
		{circle(0.71, 0.32, 0.09), hex(0x6d5545)},
		{circle(0.5, 0.56, 0.32), hex(0x8b6f5c)},             // head
		{rrect{0.33, 0.58, 0.67, 0.84, 0.13}, hex(0xb59a86)}, // muzzle
		{circle(0.39, 0.47, 0.04), hex(0x11111b)},            // eyes
		{circle(0.61, 0.47, 0.04), hex(0x11111b)},
		{circle(0.4, 0.455, 0.012), hex(0xffffff)}, // eye shine
		{circle(0.62, 0.455, 0.012), hex(0xffffff)},
		{rrect{0.43, 0.6, 0.57, 0.69, 0.045}, hex(0xf5c2e7)},    // nose
		{rrect{0.462, 0.72, 0.494, 0.79, 0.008}, hex(0xf5f5f5)}, // teeth
		{rrect{0.506, 0.72, 0.538, 0.79, 0.008}, hex(0xf5f5f5)},
	}
	render(layers, filepath.Join(dir, "icon.png"))
}
