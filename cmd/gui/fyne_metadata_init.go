package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

//go:embed icon.png
var embeddedIconPNG []byte

func init() {
	app.SetMetadata(fyne.AppMetadata{
		ID:      "com.twin.networktoolkit",
		Name:    "NetProbe",
		Version: "2.0.0",
		Build:   2,
		Icon: &fyne.StaticResource{
			StaticName:    "icon.png",
			StaticContent: embeddedIconPNG,
		},
		Release: true,
	})
}
