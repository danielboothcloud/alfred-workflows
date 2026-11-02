// Command genicons renders the workflow icons as PNGs.
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

type shape interface {
	// inside reports whether point (x, y) in [0,1] units is covered.
	inside(x, y float64) bool
}

type rrect struct{ x0, y0, x1, y1, r float64 }

func (s rrect) inside(x, y float64) bool {
	if x < s.x0 || x > s.x1 || y < s.y0 || y > s.y1 {
		return false
	}
	cx := math.Max(s.x0+s.r, math.Min(x, s.x1-s.r))
	cy := math.Max(s.y0+s.r, math.Min(y, s.y1-s.r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= s.r*s.r
}

type circle struct{ cx, cy, r float64 }

func (c circle) inside(x, y float64) bool { return (x-c.cx)*(x-c.cx)+(y-c.cy)*(y-c.cy) <= c.r*c.r }

// seg is a line segment with round caps.
type seg struct{ x0, y0, x1, y1, w float64 }

func (s seg) inside(x, y float64) bool {
	dx, dy := s.x1-s.x0, s.y1-s.y0
	t := ((x-s.x0)*dx + (y-s.y0)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	px, py := s.x0+t*dx, s.y0+t*dy
	return math.Hypot(x-px, y-py) <= s.w/2
}

type layer struct {
	s shape
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
					// Painter's algorithm: last covering layer wins.
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

var (
	bg     = hex(0x1e1e2e)
	green  = hex(0xa6e3a1)
	orange = hex(0xfab387)
	blue   = hex(0x89b4fa)
	purple = hex(0xcba6f7)
	red    = hex(0xf38ba8)
	grey   = hex(0x7f849c)
	white  = hex(0xffffff)
)

// panes draws the three-pane glyph (one tall pane left, two stacked right).
func panes(x0, y0, x1, y1, gap, r float64, left, topRight, bottomRight color.NRGBA) []layer {
	mx := (x0 + x1) / 2
	my := (y0 + y1) / 2
	return []layer{
		{rrect{x0, y0, mx - gap/2, y1, r}, left},
		{rrect{mx + gap/2, y0, x1, my - gap/2, r}, topRight},
		{rrect{mx + gap/2, my + gap/2, x1, y1, r}, bottomRight},
	}
}

func badge(c color.NRGBA, glyph ...layer) []layer {
	l := []layer{{circle{0.5, 0.5, 0.46}, c}}
	return append(l, glyph...)
}

func main() {
	dir := "workflow"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	icons := filepath.Join(dir, "icons")
	if err := os.MkdirAll(icons, 0o755); err != nil {
		log.Fatal(err)
	}

	// Workflow icon: dark tile with coloured panes.
	render(append([]layer{{rrect{0.04, 0.04, 0.96, 0.96, 0.2}, bg}},
		panes(0.18, 0.18, 0.82, 0.82, 0.07, 0.06, green, orange, blue)...),
		filepath.Join(dir, "icon.png"))

	// Session states.
	render(panes(0.1, 0.14, 0.9, 0.86, 0.08, 0.07, green, green, green), filepath.Join(icons, "attached.png"))
	dim := color.NRGBA{R: green.R, G: green.G, B: green.B, A: 150}
	render(panes(0.1, 0.14, 0.9, 0.86, 0.08, 0.07, dim, dim, dim), filepath.Join(icons, "running.png"))
	g := color.NRGBA{R: grey.R, G: grey.G, B: grey.B, A: 140}
	render(panes(0.1, 0.14, 0.9, 0.86, 0.08, 0.07, g, g, g), filepath.Join(icons, "exited.png"))

	// Actions.
	render(badge(blue,
		layer{seg{0.5, 0.28, 0.5, 0.72, 0.1}, white},
		layer{seg{0.28, 0.5, 0.72, 0.5, 0.1}, white}), filepath.Join(icons, "new.png"))
	render(badge(red,
		layer{seg{0.33, 0.33, 0.67, 0.67, 0.1}, white},
		layer{seg{0.67, 0.33, 0.33, 0.67, 0.1}, white}), filepath.Join(icons, "kill.png"))
	render(badge(red,
		layer{rrect{0.34, 0.36, 0.66, 0.74, 0.04}, white},
		layer{seg{0.28, 0.32, 0.72, 0.32, 0.06}, white},
		layer{seg{0.44, 0.25, 0.56, 0.25, 0.06}, white},
		layer{seg{0.44, 0.45, 0.44, 0.66, 0.04}, red},
		layer{seg{0.56, 0.45, 0.56, 0.66, 0.04}, red}), filepath.Join(icons, "delete.png"))
	render(badge(orange,
		layer{seg{0.33, 0.67, 0.64, 0.36, 0.12}, white},
		layer{seg{0.68, 0.32, 0.70, 0.30, 0.12}, white},
		layer{seg{0.27, 0.73, 0.30, 0.70, 0.06}, white}), filepath.Join(icons, "rename.png"))
	render(append([]layer{{circle{0.5, 0.5, 0.46}, purple}},
		panes(0.27, 0.29, 0.73, 0.71, 0.06, 0.04, white, white, white)...), filepath.Join(icons, "layout.png"))
	render(badge(red,
		layer{seg{0.5, 0.26, 0.5, 0.58, 0.11}, white},
		layer{circle{0.5, 0.72, 0.065}, white}), filepath.Join(icons, "error.png"))
}
