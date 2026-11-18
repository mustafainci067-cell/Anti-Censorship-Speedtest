package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func generateAppIconPNG() []byte {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	bg := color.NRGBA{R: 15, G: 23, B: 42, A: 255}
	border := color.NRGBA{R: 56, G: 189, B: 248, A: 255}
	cyan := color.NRGBA{R: 14, G: 165, B: 233, A: 255}
	indigo := color.NRGBA{R: 129, G: 140, B: 248, A: 255}
	emerald := color.NRGBA{R: 16, G: 185, B: 129, A: 255}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if x < 2 || x >= size-2 || y < 2 || y >= size-2 {
				img.SetNRGBA(x, y, border)
			} else {
				img.SetNRGBA(x, y, bg)
			}
		}
	}

	// Draw 4 telemetry bars inside the icon
	drawRect := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if x >= 0 && x < size && y >= 0 && y < size {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}

	drawRect(12, 38, 20, 52, emerald)
	drawRect(23, 28, 31, 52, cyan)
	drawRect(34, 18, 42, 52, cyan)
	drawRect(45, 12, 53, 52, indigo)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func init() {
	app.SetMetadata(fyne.AppMetadata{
		ID:      "com.twin.networktoolkit",
		Name:    "NetProbe",
		Version: "2.0.0",
		Build:   2,
		Icon: &fyne.StaticResource{
			StaticName:    "Icon.png",
			StaticContent: generateAppIconPNG(),
		},
		Release: true,
	})
}
