package main

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"speedtest-cli/internal/network"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	colBgPrimary    = color.NRGBA{R: 11, G: 15, B: 23, A: 255}   // #0B0F17
	colBgCard       = color.NRGBA{R: 17, G: 24, B: 39, A: 255}   // #111827
	colBgHeader     = color.NRGBA{R: 15, G: 23, B: 42, A: 255}   // #0F172A
	colBgTrack      = color.NRGBA{R: 30, G: 41, B: 59, A: 255}   // #1E293B
	colBorder       = color.NRGBA{R: 38, G: 50, B: 72, A: 255}   // #263248
	colBorderActive = color.NRGBA{R: 14, G: 165, B: 233, A: 180} // #0EA5E9
	colTextPrimary  = color.NRGBA{R: 248, G: 250, B: 252, A: 255}
	colTextMuted    = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	colTextSubtle   = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
	colAccentCyan   = color.NRGBA{R: 56, G: 189, B: 248, A: 255}  // #38BDF8
	colAccentIndigo = color.NRGBA{R: 129, G: 140, B: 248, A: 255} // #818CF8
	colAccentGreen  = color.NRGBA{R: 16, G: 185, B: 129, A: 255}  // #10B981
	colAccentAmber  = color.NRGBA{R: 245, G: 158, B: 11, A: 255}  // #F59E0B
	colAccentRed    = color.NRGBA{R: 239, G: 68, B: 68, A: 255}   // #EF4444
)

type netprobeTheme struct {
	fyne.Theme
}

func newNetprobeTheme() fyne.Theme {
	return &netprobeTheme{Theme: theme.DefaultTheme()}
}

func (t *netprobeTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colBgPrimary
	case theme.ColorNameButton:
		return color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 20, G: 28, B: 42, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 14, G: 165, B: 233, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 22, G: 31, B: 48, A: 255}
	case theme.ColorNameInputBorder:
		return colBorder
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return colBgCard
	case theme.ColorNameForeground:
		return colTextPrimary
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return colTextSubtle
	case theme.ColorNameSeparator:
		return colBorder
	default:
		return t.Theme.Color(name, theme.VariantDark)
	}
}

func (t *netprobeTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameInputRadius:
		return 6
	case theme.SizeNameText:
		return 13
	default:
		return t.Theme.Size(name)
	}
}

// slimBar is a custom smooth progress bar drawn via canvas rectangles.
type slimBar struct {
	track     *canvas.Rectangle
	fill      *canvas.Rectangle
	container *fyne.Container
	ratio     float64
}

func newSlimBar(fillColor color.Color) *slimBar {
	track := canvas.NewRectangle(colBgTrack)
	track.CornerRadius = 3
	track.SetMinSize(fyne.NewSize(10, 6))

	fill := canvas.NewRectangle(fillColor)
	fill.CornerRadius = 3

	b := &slimBar{
		track: track,
		fill:  fill,
		ratio: 0,
	}
	b.container = container.New(&slimBarLayout{bar: b}, track, fill)
	return b
}

func (b *slimBar) SetRatio(r float64) {
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	b.ratio = r
	b.container.Refresh()
}

type slimBarLayout struct {
	bar *slimBar
}

func (l *slimBarLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(size)

	fillW := size.Width * float32(l.bar.ratio)
	if fillW < 0 {
		fillW = 0
	}
	objects[1].Move(fyne.NewPos(0, 0))
	objects[1].Resize(fyne.NewSize(fillW, size.Height))
}

func (l *slimBarLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(40, 6)
}

func buildCard(borderCol color.Color, inner fyne.CanvasObject) (*canvas.Rectangle, *fyne.Container) {
	bg := canvas.NewRectangle(colBgCard)
	bg.CornerRadius = 8
	bg.StrokeColor = borderCol
	bg.StrokeWidth = 1

	padded := container.New(layout.NewCustomPaddedLayout(12, 12, 14, 14), inner)
	return bg, container.NewStack(bg, padded)
}

type nodeRowWidgets struct {
	nameText     *canvas.Text
	endpointText *canvas.Text
	rttText      *canvas.Text
	minMaxText   *canvas.Text
	jitterText   *canvas.Text
	lossText     *canvas.Text
	statusDot    *canvas.Circle
	statusText   *canvas.Text
}

func newText(str string, size float32, col color.Color, bold bool, mono bool) *canvas.Text {
	t := canvas.NewText(str, col)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold, Monospace: mono}
	return t
}

func main() {
	myApp := app.NewWithID("com.twin.networktoolkit")
	myApp.Settings().SetTheme(newNetprobeTheme())

	myWindow := myApp.NewWindow("NetProbe // Anti-Censorship Speedtest & Diagnostic Suite")
	myWindow.Resize(fyne.NewSize(840, 580))
	myWindow.SetMaster()

	var (
		selectedMode = network.ModeAuto
		currentMeta  network.ConnectionMeta
		ipRevealed   bool
		runningMu    sync.Mutex
		isRunning    bool
		cancelTest   context.CancelFunc

		lastNodes      []network.PingResult
		lastBestPing   network.PingResult
		lastDownResult network.TransferResult
		lastUpResult   network.TransferResult
	)

	// --- TOP HEADER BAR ---
	brandTitle := newText("NETPROBE", 19, colTextPrimary, true, true)
	brandTag := newText("ANTI-CENSORSHIP NETWORK DIAGNOSTIC", 11, colAccentCyan, true, true)
	brandSub := newText("Encrypted DoH Resolution  •  Direct Edge Routing  •  Multi-Stream TLS 1.3", 11, colTextSubtle, false, false)

	headerLeft := container.NewVBox(
		container.NewHBox(brandTitle, widget.NewLabel(" "), brandTag),
		brandSub,
	)

	statusDot := canvas.NewCircle(colAccentGreen)
	statusBadgeText := newText("READY", 11, colAccentGreen, true, true)
	statusDotWrap := container.NewGridWrap(fyne.NewSize(8, 8), statusDot)
	statusPillInner := container.NewHBox(
		container.NewCenter(statusDotWrap),
		statusBadgeText,
	)
	statusPillBg := canvas.NewRectangle(colBgTrack)
	statusPillBg.CornerRadius = 6
	statusPillBg.StrokeColor = colBorder
	statusPillBg.StrokeWidth = 1
	statusPill := container.NewStack(
		statusPillBg,
		container.New(layout.NewCustomPaddedLayout(5, 5, 10, 10), statusPillInner),
	)

	modeOptions := []string{
		"Auto Bypass (DoH + Edge IP)",
		"Force DoH (1.1.1.1 / 8.8.8.8)",
		"Direct Edge IP (Zero DNS)",
		"System DNS Resolver",
	}
	modeSelect := widget.NewSelect(modeOptions, func(val string) {
		switch val {
		case modeOptions[1]:
			selectedMode = network.ModeForceDoH
		case modeOptions[2]:
			selectedMode = network.ModeDirectIP
		case modeOptions[3]:
			selectedMode = network.ModeSystemDNS
		default:
			selectedMode = network.ModeAuto
		}
	})
	modeSelect.Selected = modeOptions[0]

	headerRight := container.NewHBox(
		container.NewCenter(newText("ROUTE MODE:", 10, colTextSubtle, true, true)),
		modeSelect,
		container.NewCenter(statusPill),
	)

	headerBg := canvas.NewRectangle(colBgHeader)
	headerBg.StrokeColor = colBorder
	headerBg.StrokeWidth = 1
	headerBar := container.NewStack(
		headerBg,
		container.New(layout.NewCustomPaddedLayout(12, 12, 18, 18),
			container.NewBorder(nil, nil, headerLeft, headerRight),
		),
	)

	// --- METADATA TELEMETRY STRIP ---
	metaEdgeVal := newText("Discovering...", 12, colTextPrimary, true, true)
	metaASNVal := newText("--", 12, colTextPrimary, true, true)
	metaIPVal := newText("---.---.xxx.xxx", 12, colTextPrimary, true, true)
	metaRouteVal := newText("DoH + TLS 1.3 (TCP/443)", 12, colAccentCyan, true, true)

	var toggleIPBtn *widget.Button
	toggleIPBtn = widget.NewButton("Show", func() {
		ipRevealed = !ipRevealed
		if ipRevealed {
			if currentMeta.ClientIP != "" {
				metaIPVal.Text = currentMeta.ClientIP
			}
			toggleIPBtn.SetText("Mask")
		} else {
			metaIPVal.Text = currentMeta.MaskedIP()
			toggleIPBtn.SetText("Show")
		}
		metaIPVal.Refresh()
	})
	toggleIPBtn.Importance = widget.LowImportance

	buildMetaItem := func(label string, valObj fyne.CanvasObject) *fyne.Container {
		lbl := newText(label, 10, colTextSubtle, true, true)
		return container.NewVBox(lbl, valObj)
	}

	ipRow := container.NewHBox(container.NewCenter(metaIPVal), toggleIPBtn)
	metaGrid := container.NewGridWithColumns(4,
		buildMetaItem("EDGE POP / LOCATION", metaEdgeVal),
		buildMetaItem("NETWORK / ASN", metaASNVal),
		buildMetaItem("CLIENT PUBLIC IP", ipRow),
		buildMetaItem("BYPASS TRANSPORT", metaRouteVal),
	)
	_, metaStripCard := buildCard(colBorder, metaGrid)

	// --- 4 PRIMARY METRIC CARDS ---
	// 1. Download Card
	downTitle := newText("DOWNLOAD THROUGHPUT", 10, colTextMuted, true, true)
	downVal := newText("0.00", 30, colAccentCyan, true, true)
	downUnit := newText("Mbps", 13, colTextMuted, true, false)
	downSub := newText("Peak: -- Mbps  •  Transferred: 0.0 MB  •  4x TLS Streams", 11, colTextSubtle, false, false)
	downBar := newSlimBar(colAccentCyan)

	downCardBg, downCard := buildCard(colBorder, container.NewVBox(
		downTitle,
		container.NewHBox(downVal, container.NewBorder(nil, downUnit, nil, nil)),
		downSub,
		container.New(layout.NewCustomPaddedLayout(6, 0, 0, 0), downBar.container),
	))

	// 2. Upload Card
	upTitle := newText("UPLOAD THROUGHPUT", 10, colTextMuted, true, true)
	upVal := newText("0.00", 30, colAccentIndigo, true, true)
	upUnit := newText("Mbps", 13, colTextMuted, true, false)
	upSub := newText("Peak: -- Mbps  •  Transferred: 0.0 MB  •  3x TLS Streams", 11, colTextSubtle, false, false)
	upBar := newSlimBar(colAccentIndigo)

	upCardBg, upCard := buildCard(colBorder, container.NewVBox(
		upTitle,
		container.NewHBox(upVal, container.NewBorder(nil, upUnit, nil, nil)),
		upSub,
		container.New(layout.NewCustomPaddedLayout(6, 0, 0, 0), upBar.container),
	))

	// 3. Latency & Jitter Card
	pingTitle := newText("UNLOADED LATENCY (TCP:443)", 10, colTextMuted, true, true)
	pingVal := newText("--.-", 26, colAccentGreen, true, true)
	pingUnit := newText("ms", 13, colTextMuted, true, false)
	pingSub := newText("Jitter: --.- ms  •  Min / Max: --.- / --.- ms", 11, colTextSubtle, false, false)

	pingCardBg, pingCard := buildCard(colBorder, container.NewVBox(
		pingTitle,
		container.NewHBox(pingVal, container.NewBorder(nil, pingUnit, nil, nil)),
		pingSub,
	))

	// 4. Link Quality & Loss Card
	gradeTitle := newText("PACKET INTEGRITY & QUALITY", 10, colTextMuted, true, true)
	gradeVal := newText("STANDBY", 22, colTextPrimary, true, true)
	gradeLossBadge := newText("Loss: 0.0%", 12, colAccentGreen, true, true)
	gradeSub := newText("Start diagnostic to evaluate streaming, VoIP & gaming grade", 11, colTextSubtle, false, false)

	_, gradeCard := buildCard(colBorder, container.NewVBox(
		gradeTitle,
		container.NewHBox(gradeVal, layout.NewSpacer(), container.NewCenter(gradeLossBadge)),
		gradeSub,
	))

	topCardsGrid := container.NewGridWithColumns(2, downCard, upCard)
	midCardsGrid := container.NewGridWithColumns(2, pingCard, gradeCard)

	// --- BACKBONE REACHABILITY MATRIX ---
	matrixHeader := container.NewBorder(nil, nil,
		newText("BACKBONE REACHABILITY MATRIX (TCP/443 ANTI-ICMP-BLOCK PROBES)", 10, colTextMuted, true, true),
		newText("6x CONCURRENT SYN/ACK HANDSHAKES PER NODE", 10, colTextSubtle, false, true),
	)

	colHeaders := container.NewGridWithColumns(6,
		newText("BACKBONE NODE", 10, colTextSubtle, true, true),
		newText("ENDPOINT", 10, colTextSubtle, true, true),
		newText("AVG RTT", 10, colTextSubtle, true, true),
		newText("MIN / MAX", 10, colTextSubtle, true, true),
		newText("JITTER / LOSS", 10, colTextSubtle, true, true),
		newText("STATUS", 10, colTextSubtle, true, true),
	)

	nodeRows := make([]nodeRowWidgets, len(network.DefaultBackboneNodes))
	nodeRowContainers := make([]fyne.CanvasObject, 0, len(network.DefaultBackboneNodes)+3)
	nodeRowContainers = append(nodeRowContainers,
		matrixHeader,
		widget.NewSeparator(),
		colHeaders,
	)

	for i, n := range network.DefaultBackboneNodes {
		w := nodeRowWidgets{
			nameText:     newText(n.Name, 12, colTextPrimary, true, false),
			endpointText: newText(fmt.Sprintf("%s:443", n.Endpoint), 12, colTextMuted, false, true),
			rttText:      newText("--.- ms", 12, colTextPrimary, true, true),
			minMaxText:   newText("--.- / --.- ms", 11, colTextSubtle, false, true),
			jitterText:   newText("--.- ms  (0%)", 11, colTextSubtle, false, true),
			statusDot:    canvas.NewCircle(colTextSubtle),
			statusText:   newText("STANDBY", 11, colTextSubtle, true, true),
		}
		nodeRows[i] = w

		dotWrap := container.NewGridWrap(fyne.NewSize(7, 7), w.statusDot)
		statusCell := container.NewHBox(container.NewCenter(dotWrap), w.statusText)

		rowGrid := container.NewGridWithColumns(6,
			w.nameText,
			w.endpointText,
			w.rttText,
			w.minMaxText,
			w.jitterText,
			statusCell,
		)
		nodeRowContainers = append(nodeRowContainers, rowGrid)
	}

	_, matrixCard := buildCard(colBorder, container.NewVBox(nodeRowContainers...))

	// --- BOTTOM ACTION & STATUS BAR ---
	statusLine := newText("Ready — Select routing mode or click Start Diagnostic", 12, colTextMuted, false, false)

	var copyBtn *widget.Button
	copyBtn = widget.NewButtonWithIcon("Copy Report", theme.ContentCopyIcon(), func() {
		report := fmt.Sprintf(
			"NETPROBE DIAGNOSTIC REPORT\n"+
				"==========================\n"+
				"Timestamp   : %s\n"+
				"Edge PoP    : %s\n"+
				"Network ASN : %s\n"+
				"Transport   : %s (%s)\n"+
				"Latency     : %.1f ms (Jitter: %.2f ms, Loss: %.1f%%)\n"+
				"Download    : %.2f Mbps (Peak: %.2f Mbps, %.1f MB)\n"+
				"Upload      : %.2f Mbps (Peak: %.2f Mbps, %.1f MB)\n",
			time.Now().Format("2006-01-02 15:04:05"),
			currentMeta.LocationSummary(),
			currentMeta.ASN,
			currentMeta.BypassMethod,
			currentMeta.TLSVersion,
			lastBestPing.PingMs,
			lastBestPing.JitterMs,
			lastBestPing.LossPercent,
			lastDownResult.SustainedMbps,
			lastDownResult.PeakMbps,
			float64(lastDownResult.BytesTransferred)/(1024*1024),
			lastUpResult.SustainedMbps,
			lastUpResult.PeakMbps,
			float64(lastUpResult.BytesTransferred)/(1024*1024),
		)
		if len(lastNodes) > 0 {
			report += "\nBackbone Nodes:\n"
			for _, nr := range lastNodes {
				if nr.Success {
					report += fmt.Sprintf("  - %-18s (%s:%d): %.1f ms | Jitter: %.2f ms | Loss: %.1f%%\n",
						nr.Name, nr.Endpoint, nr.Port, nr.PingMs, nr.JitterMs, nr.LossPercent)
				} else {
					report += fmt.Sprintf("  - %-18s (%s:%d): UNREACHABLE\n", nr.Name, nr.Endpoint, nr.Port)
				}
			}
		}
		myWindow.Clipboard().SetContent(report)
		copyBtn.SetText("Copied")
		go func() {
			time.Sleep(1800 * time.Millisecond)
			fyne.Do(func() {
				copyBtn.SetText("Copy Report")
			})
		}()
	})

	setStatusBadge := func(label string, col color.Color) {
		statusDot.FillColor = col
		statusDot.Refresh()
		statusBadgeText.Text = label
		statusBadgeText.Color = col
		statusBadgeText.Refresh()
	}

	applyMetaToUI := func(meta network.ConnectionMeta) {
		currentMeta = meta
		metaEdgeVal.Text = meta.LocationSummary()
		metaEdgeVal.Refresh()

		if meta.ASN != "" {
			metaASNVal.Text = meta.ASN
		} else {
			metaASNVal.Text = "Detected"
		}
		metaASNVal.Refresh()

		if ipRevealed && meta.ClientIP != "" {
			metaIPVal.Text = meta.ClientIP
		} else {
			metaIPVal.Text = meta.MaskedIP()
		}
		metaIPVal.Refresh()

		routeStr := meta.BypassMethod
		if meta.TLSVersion != "" && !strings.Contains(routeStr, "TLS") {
			routeStr += " • " + meta.TLSVersion
		}
		metaRouteVal.Text = routeStr
		metaRouteVal.Refresh()
	}

	// Background initial metadata discovery on launch
	go func() {
		meta := network.FetchConnectionMeta(context.Background(), selectedMode)
		fyne.Do(func() {
			applyMetaToUI(meta)
		})
	}()

	var runButton *widget.Button
	runButton = widget.NewButtonWithIcon("Start Diagnostic", theme.MediaPlayIcon(), func() {
		runningMu.Lock()
		if isRunning {
			if cancelTest != nil {
				cancelTest()
			}
			runningMu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelTest = cancel
		isRunning = true
		mode := selectedMode
		runningMu.Unlock()

		// Prepare UI for active test
		runButton.SetText("Stop Test")
		runButton.SetIcon(theme.MediaStopIcon())
		modeSelect.Disable()

		setStatusBadge("PROBING", colAccentCyan)
		downVal.Text = "0.00"
		downVal.Refresh()
		downSub.Text = "Waiting for latency probe..."
		downSub.Refresh()
		downBar.SetRatio(0)

		upVal.Text = "0.00"
		upVal.Refresh()
		upSub.Text = "Queued..."
		upSub.Refresh()
		upBar.SetRatio(0)

		gradeVal.Text = "TESTING..."
		gradeVal.Color = colAccentCyan
		gradeVal.Refresh()
		gradeSub.Text = "Evaluating backbone reachability & throughput..."
		gradeSub.Refresh()

		pingCardBg.StrokeColor = colBorderActive
		pingCardBg.Refresh()

		statusLine.Text = "Phase 1/3 — Probing 4 global backbone nodes over TCP/443 & discovering edge metadata..."
		statusLine.Refresh()

		for i := range nodeRows {
			nodeRows[i].statusDot.FillColor = colAccentCyan
			nodeRows[i].statusDot.Refresh()
			nodeRows[i].statusText.Text = "PROBING"
			nodeRows[i].statusText.Color = colAccentCyan
			nodeRows[i].statusText.Refresh()
		}

		go func() {
			defer func() {
				runningMu.Lock()
				isRunning = false
				cancelTest = nil
				runningMu.Unlock()

				fyne.Do(func() {
					runButton.SetText("Start Diagnostic")
					runButton.SetIcon(theme.MediaPlayIcon())
					modeSelect.Enable()
					pingCardBg.StrokeColor = colBorder
					pingCardBg.Refresh()
					downCardBg.StrokeColor = colBorder
					downCardBg.Refresh()
					upCardBg.StrokeColor = colBorder
					upCardBg.Refresh()
				})
			}()

			// Phase 1: Measure Unloaded Multi-Node TCP Latency first, then Edge Metadata
			nodes, bestPing := network.MeasureAllNodes(ctx)
			meta := network.FetchConnectionMeta(ctx, mode)

			if ctx.Err() != nil {
				fyne.Do(func() {
					setStatusBadge("ABORTED", colAccentAmber)
					gradeVal.Text = "CANCELLED"
					gradeVal.Color = colAccentAmber
					gradeVal.Refresh()
					statusLine.Text = "Diagnostic stopped by user."
					statusLine.Refresh()
				})
				return
			}

			lastNodes = nodes
			lastBestPing = bestPing

			fyne.Do(func() {
				applyMetaToUI(meta)
				pingCardBg.StrokeColor = colBorder
				pingCardBg.Refresh()

				if bestPing.Success {
					pingVal.Text = fmt.Sprintf("%.1f", bestPing.PingMs)
					pingVal.Color = colAccentGreen
					pingSub.Text = fmt.Sprintf("Jitter: %.2f ms  •  Min / Max: %.1f / %.1f ms  (%s)",
						bestPing.JitterMs, bestPing.MinMs, bestPing.MaxMs, bestPing.Name)
					gradeLossBadge.Text = fmt.Sprintf("Loss: %.1f%%", bestPing.LossPercent)
				} else {
					pingVal.Text = "ERR"
					pingVal.Color = colAccentRed
					pingSub.Text = "All backbone TCP probes timed out"
					gradeLossBadge.Text = "Loss: 100%"
				}
				pingVal.Refresh()
				pingSub.Refresh()
				gradeLossBadge.Refresh()

				for i, r := range nodes {
					if i >= len(nodeRows) {
						continue
					}
					w := nodeRows[i]
					w.endpointText.Text = fmt.Sprintf("%s:%d", r.Endpoint, r.Port)
					w.endpointText.Refresh()
					if r.Success {
						w.rttText.Text = fmt.Sprintf("%.1f ms", r.PingMs)
						w.minMaxText.Text = fmt.Sprintf("%.1f / %.1f ms", r.MinMs, r.MaxMs)
						w.jitterText.Text = fmt.Sprintf("%.2f ms  (%.0f%%)", r.JitterMs, r.LossPercent)
						w.statusDot.FillColor = colAccentGreen
						w.statusText.Text = "ONLINE"
						w.statusText.Color = colAccentGreen
					} else {
						w.rttText.Text = "--"
						w.minMaxText.Text = "-- / --"
						w.jitterText.Text = "-- (100%)"
						w.statusDot.FillColor = colAccentRed
						w.statusText.Text = "FILTERED"
						w.statusText.Color = colAccentRed
					}
					w.rttText.Refresh()
					w.minMaxText.Refresh()
					w.jitterText.Refresh()
					w.statusDot.Refresh()
					w.statusText.Refresh()
				}

				// Highlight Download Card for Phase 2
				setStatusBadge("DOWNLOADING", colAccentCyan)
				downCardBg.StrokeColor = colBorderActive
				downCardBg.Refresh()
				statusLine.Text = "Phase 2/3 — Benchmarking multi-stream download bandwidth over TLS 1.3..."
				statusLine.Refresh()
			})

			// Phase 2: Download Test
			downOpts := network.DefaultDownloadOptions()
			downOpts.Mode = mode
			downRes := network.TestDownloadWithContext(ctx, downOpts, func(t network.TransferTelemetry) {
				if ctx.Err() != nil {
					return
				}
				mbTransferred := float64(t.BytesTransferred) / (1024 * 1024)
				fyne.Do(func() {
					downVal.Text = fmt.Sprintf("%.2f", t.CurrentMbps)
					downVal.Refresh()
					downSub.Text = fmt.Sprintf("Peak: %.2f Mbps  •  Volume: %.1f MB  •  %dx Streams",
						t.PeakMbps, mbTransferred, t.ActiveStreams)
					downSub.Refresh()
					downBar.SetRatio(t.Progress)
				})
			})

			if ctx.Err() != nil {
				fyne.Do(func() {
					setStatusBadge("ABORTED", colAccentAmber)
					gradeVal.Text = "CANCELLED"
					gradeVal.Color = colAccentAmber
					gradeVal.Refresh()
					statusLine.Text = "Diagnostic stopped by user during download phase."
					statusLine.Refresh()
				})
				return
			}

			lastDownResult = downRes

			fyne.Do(func() {
				downCardBg.StrokeColor = colBorder
				downCardBg.Refresh()
				downVal.Text = fmt.Sprintf("%.2f", downRes.SustainedMbps)
				downVal.Refresh()
				downSub.Text = fmt.Sprintf("Peak: %.2f Mbps  •  Volume: %.1f MB  •  %dx Streams",
					downRes.PeakMbps, float64(downRes.BytesTransferred)/(1024*1024), downRes.Streams)
				downSub.Refresh()
				downBar.SetRatio(1.0)

				setStatusBadge("UPLOADING", colAccentIndigo)
				upCardBg.StrokeColor = colAccentIndigo
				upCardBg.Refresh()
				statusLine.Text = "Phase 3/3 — Benchmarking multi-stream upload bandwidth over TLS 1.3..."
				statusLine.Refresh()
			})

			// Phase 3: Upload Test
			upOpts := network.DefaultUploadOptions()
			upOpts.Mode = mode
			upRes := network.TestUploadWithContext(ctx, upOpts, func(t network.TransferTelemetry) {
				if ctx.Err() != nil {
					return
				}
				mbTransferred := float64(t.BytesTransferred) / (1024 * 1024)
				fyne.Do(func() {
					upVal.Text = fmt.Sprintf("%.2f", t.CurrentMbps)
					upVal.Refresh()
					upSub.Text = fmt.Sprintf("Peak: %.2f Mbps  •  Volume: %.1f MB  •  %dx Streams",
						t.PeakMbps, mbTransferred, t.ActiveStreams)
					upSub.Refresh()
					upBar.SetRatio(t.Progress)
				})
			})

			if ctx.Err() != nil {
				fyne.Do(func() {
					setStatusBadge("ABORTED", colAccentAmber)
					gradeVal.Text = "CANCELLED"
					gradeVal.Color = colAccentAmber
					gradeVal.Refresh()
					statusLine.Text = "Diagnostic stopped by user during upload phase."
					statusLine.Refresh()
				})
				return
			}

			lastUpResult = upRes
			grade, gradeDesc := network.GradeConnection(
				bestPing.PingMs,
				bestPing.JitterMs,
				bestPing.LossPercent,
				downRes.SustainedMbps,
				upRes.SustainedMbps,
			)

			fyne.Do(func() {
				upCardBg.StrokeColor = colBorder
				upCardBg.Refresh()
				upVal.Text = fmt.Sprintf("%.2f", upRes.SustainedMbps)
				upVal.Refresh()
				upSub.Text = fmt.Sprintf("Peak: %.2f Mbps  •  Volume: %.1f MB  •  %dx Streams",
					upRes.PeakMbps, float64(upRes.BytesTransferred)/(1024*1024), upRes.Streams)
				upSub.Refresh()
				upBar.SetRatio(1.0)

				gradeVal.Text = grade
				if strings.HasPrefix(grade, "OPTIMAL") || strings.HasPrefix(grade, "GOOD") {
					gradeVal.Color = colAccentGreen
				} else if strings.HasPrefix(grade, "STABLE") {
					gradeVal.Color = colAccentCyan
				} else {
					gradeVal.Color = colAccentAmber
				}
				gradeVal.Refresh()
				gradeSub.Text = gradeDesc
				gradeSub.Refresh()

				if downRes.BypassMethod != "" {
					currentMeta.BypassMethod = downRes.BypassMethod
					applyMetaToUI(currentMeta)
				}

				setStatusBadge("COMPLETE", colAccentGreen)
				statusLine.Text = fmt.Sprintf("Diagnostic complete — Down: %.2f Mbps  |  Up: %.2f Mbps  |  RTT: %.1f ms",
					downRes.SustainedMbps, upRes.SustainedMbps, bestPing.PingMs)
				statusLine.Refresh()
			})
		}()
	})
	runButton.Importance = widget.HighImportance

	bottomButtons := container.NewHBox(copyBtn, runButton)
	bottomBarBg := canvas.NewRectangle(colBgHeader)
	bottomBarBg.StrokeColor = colBorder
	bottomBarBg.StrokeWidth = 1
	bottomBar := container.NewStack(
		bottomBarBg,
		container.New(layout.NewCustomPaddedLayout(10, 10, 18, 18),
			container.NewBorder(nil, nil, container.NewCenter(statusLine), bottomButtons),
		),
	)

	mainBody := container.New(layout.NewCustomPaddedLayout(14, 14, 18, 18),
		container.NewVBox(
			metaStripCard,
			container.New(layout.NewCustomPaddedLayout(6, 0, 0, 0), topCardsGrid),
			container.New(layout.NewCustomPaddedLayout(6, 0, 0, 0), midCardsGrid),
			container.New(layout.NewCustomPaddedLayout(6, 0, 0, 0), matrixCard),
		),
	)

	rootLayout := container.NewBorder(
		headerBar,
		bottomBar,
		nil,
		nil,
		container.NewVScroll(mainBody),
	)

	myWindow.SetContent(rootLayout)
	myWindow.ShowAndRun()
}
