// Renders the keepty icon (rounded square, three bars) as a 1024px PNG.
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

type rrect struct{ x, y, w, h, r float64; c color.NRGBA }

func (q rrect) hit(px, py float64) bool {
	if px < q.x || py < q.y || px > q.x+q.w || py > q.y+q.h { return false }
	cx := px; cy := py
	if px < q.x+q.r { cx = q.x + q.r } else if px > q.x+q.w-q.r { cx = q.x + q.w - q.r }
	if py < q.y+q.r { cy = q.y + q.r } else if py > q.y+q.h-q.r { cy = q.y + q.h - q.r }
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= q.r*q.r
}

func main() {
	out, ss := 1024, 4
	shapes := []rrect{
		{0, 0, 128, 128, 28, color.NRGBA{0x0f, 0x11, 0x15, 255}},
		{24, 30, 80, 12, 6, color.NRGBA{0x3b, 0x82, 0xf6, 255}},
		{24, 58, 56, 12, 6, color.NRGBA{0xf5, 0x9e, 0x0b, 255}},
		{24, 86, 68, 12, 6, color.NRGBA{0x22, 0xc5, 0x5e, 255}},
	}
	// "tray": a macOS template image, black bars on transparent, 44px (22pt @2x).
	if len(os.Args) > 2 && os.Args[2] == "tray" {
		out = 44
		black := color.NRGBA{0, 0, 0, 255}
		shapes = []rrect{{16, 26, 96, 14, 7, black}, {16, 57, 68, 14, 7, black}, {16, 88, 82, 14, 7, black}}
	}
	img := image.NewNRGBA(image.Rect(0, 0, out, out))
	for y := 0; y < out; y++ {
		for x := 0; x < out; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/float64(ss)) * 128 / float64(out)
					py := (float64(y) + (float64(sy)+0.5)/float64(ss)) * 128 / float64(out)
					var c *color.NRGBA
					for i := range shapes {
						if shapes[i].hit(px, py) { c = &shapes[i].c }
					}
					if c != nil { r += float64(c.R); g += float64(c.G); b += float64(c.B); a += 255 }
				}
			}
			n := float64(ss * ss)
			if a > 0 { cov := a / n / 255; img.SetNRGBA(x, y, color.NRGBA{uint8(r / (a / 255)), uint8(g / (a / 255)), uint8(b / (a / 255)), uint8(cov * 255)}) }
		}
	}
	f, _ := os.Create(os.Args[1]); defer f.Close(); _ = png.Encode(f, img)
}
