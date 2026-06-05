package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// iconBytes returns the idle icon: a hollow ring (no pending activity).
func iconBytes() []byte {
	return renderIcon(func(dist, outerR float64) bool {
		return dist <= outerR && dist >= outerR-3
	})
}

// iconActiveBytes returns the active icon: a filled circle (pending activity).
func iconActiveBytes() []byte {
	return renderIcon(func(dist, outerR float64) bool {
		return dist <= outerR
	})
}

func renderIcon(fill func(dist, outerR float64) bool) []byte {
	const size = 22
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	outerR := float64(size)/2 - 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if fill(math.Sqrt(dx*dx+dy*dy), outerR) {
				img.Set(x, y, color.NRGBA{0, 0, 0, 255})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
