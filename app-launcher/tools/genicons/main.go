// Command genicons renders the workflow icon as a PNG.
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

type rrect struct{ x0, y0, x1, y1, r float64 }

func (s rrect) inside(x, y float64) bool {
	if x < s.x0 || x > s.x1 || y < s.y0 || y > s.y1 {
		return false
	}
	cx := math.Max(s.x0+s.r, math.Min(x, s.x1-s.r))
	cy := math.Max(s.y0+s.r, math.Min(y, s.y1-s.r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= s.r*s.r
}

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
	// Dark tile with a 3x3 grid of coloured app tiles.
	colours := []color.NRGBA{
		hex(0xf38ba8), hex(0xfab387), hex(0xf9e2af),
		hex(0xa6e3a1), hex(0x94e2d5), hex(0x89b4fa),
		hex(0xcba6f7), hex(0xf5c2e7), hex(0xb4befe),
	}
	layers := []layer{{rrect{0.04, 0.04, 0.96, 0.96, 0.2}, hex(0x1e1e2e)}}
	const start, cell, gap = 0.2, 0.18, 0.06
	for i, c := range colours {
		x := start + float64(i%3)*(cell+gap)
		y := start + float64(i/3)*(cell+gap)
		layers = append(layers, layer{rrect{x, y, x + cell, y + cell, 0.05}, c})
	}
	render(layers, filepath.Join(dir, "icon.png"))
}
