package main

import (
	"fmt"
	"os"

	"speedtest-cli/internal/network"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func main() {
	// Force dark theme
	os.Setenv("FYNE_THEME", "dark")
	
	myApp := app.New()
	myWindow := myApp.NewWindow("Network Toolkit")
	myWindow.Resize(fyne.NewSize(500, 350))

	title := widget.NewLabelWithStyle("Network Toolkit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	resultsLabel := widget.NewLabel("Results will appear here.")
	resultsLabel.Wrapping = fyne.TextWrapWord
	resultsLabel.Alignment = fyne.TextAlignCenter

	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	var runButton *widget.Button

	runButton = widget.NewButtonWithIcon("▶ Run Speedtest", theme.MediaPlayIcon(), func() {
		runButton.Disable()
		progressBar.SetValue(0)
		progressBar.Show()
		resultsLabel.SetText("Testing Ping and Jitter...")

		go func() {
			endpoints := []string{"1.1.1.1", "8.8.8.8"}
			var finalPing, finalJitter int64

			for _, ep := range endpoints {
				pingRes := network.MeasurePingAndJitter(ep)
				if pingRes.Success {
					finalPing = pingRes.Ping.Milliseconds()
					finalJitter = pingRes.Jitter.Milliseconds()
					break
				}
			}

			resultsLabel.SetText("Testing Download Bandwidth...")
			downSpeed := network.TestDownload(func(percent float64) {
				progressBar.SetValue(percent)
			})

			progressBar.SetValue(0)
			resultsLabel.SetText("Testing Upload Bandwidth...")
			upSpeed := network.TestUpload(func(percent float64) {
				progressBar.SetValue(percent)
			})

			progressBar.Hide()
			
			resultStr := fmt.Sprintf(
				"✅ Diagnostic Complete\n\nPing: %d ms\nJitter: %d ms\nDownload: %.2f Mbps\nUpload: %.2f Mbps",
				finalPing, finalJitter, downSpeed, upSpeed,
			)
			resultsLabel.SetText(resultStr)
			runButton.Enable()
		}()
	})
	
	runButton.Importance = widget.HighImportance

	vpnButton := widget.NewButtonWithIcon("🔒 Connect to Homelab VPN (Coming Soon)", theme.WarningIcon(), func() {})
	vpnButton.Disable()

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		container.NewPadded(runButton),
		container.NewPadded(vpnButton),
		container.NewPadded(progressBar),
		widget.NewSeparator(),
		container.NewPadded(resultsLabel),
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}
