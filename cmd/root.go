package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"speedtest-cli/internal/network"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	jsonOutput  bool
	runOnce     bool
	bypassFlag  string
	streamCount int
	durationSec int
	program     *tea.Program

	colCyan   = lipgloss.Color("#38BDF8")
	colIndigo = lipgloss.Color("#818CF8")
	colGreen  = lipgloss.Color("#10B981")
	colAmber  = lipgloss.Color("#F59E0B")
	colRed    = lipgloss.Color("#EF4444")
	colText   = lipgloss.Color("#F8FAFC")
	colMuted  = lipgloss.Color("#94A3B8")
	colSubtle = lipgloss.Color("#64748B")
	colBorder = lipgloss.Color("#334155")

	headerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colText).
				Background(lipgloss.Color("#0EA5E9")).
				Padding(0, 1)

	headerTagStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colCyan)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colSubtle)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder).
			Padding(0, 2)

	activePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colCyan).
				Padding(0, 2)

	labelStyle = lipgloss.NewStyle().
			Foreground(colSubtle).
			Bold(true)

	valStyle = lipgloss.NewStyle().
			Foreground(colText).
			Bold(true)

	cyanValStyle = lipgloss.NewStyle().
			Foreground(colCyan).
			Bold(true)

	indigoValStyle = lipgloss.NewStyle().
			Foreground(colIndigo).
			Bold(true)

	greenValStyle = lipgloss.NewStyle().
			Foreground(colGreen).
			Bold(true)

	amberValStyle = lipgloss.NewStyle().
			Foreground(colAmber).
			Bold(true)

	redValStyle = lipgloss.NewStyle().
			Foreground(colRed).
			Bold(true)
)

type JSONResult struct {
	PingMs           int64                  `json:"ping_ms"`
	PingExactMs      float64                `json:"ping_exact_ms"`
	JitterMs         int64                  `json:"jitter_ms"`
	JitterExactMs    float64                `json:"jitter_exact_ms"`
	PacketLossPct    float64                `json:"packet_loss_pct"`
	DownloadMbps     float64                `json:"download_mbps"`
	DownloadPeakMbps float64                `json:"download_peak_mbps"`
	UploadMbps       float64                `json:"upload_mbps"`
	UploadPeakMbps   float64                `json:"upload_peak_mbps"`
	LinkGrade        string                 `json:"link_grade"`
	Connection       network.ConnectionMeta `json:"connection"`
	BackboneNodes    []network.PingResult   `json:"backbone_nodes"`
}

func parseBypassMode(raw string) network.BypassMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "doh":
		return network.ModeForceDoH
	case "direct":
		return network.ModeDirectIP
	case "system":
		return network.ModeSystemDNS
	default:
		return network.ModeAuto
	}
}

func nextBypassMode(m network.BypassMode) network.BypassMode {
	switch m {
	case network.ModeAuto:
		return network.ModeForceDoH
	case network.ModeForceDoH:
		return network.ModeDirectIP
	case network.ModeDirectIP:
		return network.ModeSystemDNS
	default:
		return network.ModeAuto
	}
}

func runJSONMode() {
	ctx := context.Background()
	mode := parseBypassMode(bypassFlag)

	nodes, bestPing := network.MeasureAllNodes(ctx)
	meta := network.FetchConnectionMeta(ctx, mode)

	downOpts := network.DefaultDownloadOptions()
	downOpts.Mode = mode
	if streamCount > 0 {
		downOpts.Streams = streamCount
	}
	if durationSec > 0 {
		downOpts.Duration = time.Duration(durationSec) * time.Second
	}
	downRes := network.TestDownloadWithContext(ctx, downOpts, nil)

	upOpts := network.DefaultUploadOptions()
	upOpts.Mode = mode
	if streamCount > 0 {
		upOpts.Streams = streamCount
	}
	if durationSec > 0 {
		upOpts.Duration = time.Duration(durationSec) * time.Second
	}
	upRes := network.TestUploadWithContext(ctx, upOpts, nil)

	if downRes.BypassMethod != "" {
		meta.BypassMethod = downRes.BypassMethod
	}
	grade, _ := network.GradeConnection(
		bestPing.PingMs,
		bestPing.JitterMs,
		bestPing.LossPercent,
		downRes.SustainedMbps,
		upRes.SustainedMbps,
	)

	result := JSONResult{
		PingMs:           bestPing.Ping.Milliseconds(),
		PingExactMs:      bestPing.PingMs,
		JitterMs:         bestPing.Jitter.Milliseconds(),
		JitterExactMs:    bestPing.JitterMs,
		PacketLossPct:    bestPing.LossPercent,
		DownloadMbps:     downRes.SustainedMbps,
		DownloadPeakMbps: downRes.PeakMbps,
		UploadMbps:       upRes.SustainedMbps,
		UploadPeakMbps:   upRes.PeakMbps,
		LinkGrade:        grade,
		Connection:       meta,
		BackboneNodes:    nodes,
	}

	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("{\"error\": \"%v\"}\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

type state int

const (
	statePing state = iota
	stateDownload
	stateUpload
	stateResult
)

type phase1DoneMsg struct {
	meta     network.ConnectionMeta
	nodes    []network.PingResult
	bestPing network.PingResult
}

type telemetryMsg struct {
	phase     state
	telemetry network.TransferTelemetry
}

type downDoneMsg struct {
	res network.TransferResult
}

type upDoneMsg struct {
	res network.TransferResult
}

func startPhase1Cmd(mode network.BypassMode) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		nodes, bestPing := network.MeasureAllNodes(ctx)
		meta := network.FetchConnectionMeta(ctx, mode)
		return phase1DoneMsg{
			meta:     meta,
			nodes:    nodes,
			bestPing: bestPing,
		}
	}
}

func startDownloadCmd(mode network.BypassMode) tea.Cmd {
	return func() tea.Msg {
		opts := network.DefaultDownloadOptions()
		opts.Mode = mode
		if streamCount > 0 {
			opts.Streams = streamCount
		}
		if durationSec > 0 {
			opts.Duration = time.Duration(durationSec) * time.Second
		}
		res := network.TestDownloadWithContext(context.Background(), opts, func(t network.TransferTelemetry) {
			if program != nil {
				program.Send(telemetryMsg{phase: stateDownload, telemetry: t})
			}
		})
		return downDoneMsg{res: res}
	}
}

func startUploadCmd(mode network.BypassMode) tea.Cmd {
	return func() tea.Msg {
		opts := network.DefaultUploadOptions()
		opts.Mode = mode
		if streamCount > 0 {
			opts.Streams = streamCount
		}
		if durationSec > 0 {
			opts.Duration = time.Duration(durationSec) * time.Second
		}
		res := network.TestUploadWithContext(context.Background(), opts, func(t network.TransferTelemetry) {
			if program != nil {
				program.Send(telemetryMsg{phase: stateUpload, telemetry: t})
			}
		})
		return upDoneMsg{res: res}
	}
}

type model struct {
	state       state
	mode        network.BypassMode
	spinner     spinner.Model
	progressBar progress.Model

	meta          network.ConnectionMeta
	nodes         []network.PingResult
	bestPing      network.PingResult
	liveTelemetry network.TransferTelemetry
	downRes       network.TransferResult
	upRes         network.TransferResult

	quitting bool
}

func initialModel(mode network.BypassMode) model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(colCyan)

	prog := progress.New(progress.WithGradient("#0EA5E9", "#818CF8"))
	prog.Width = 54

	return model{
		state:       statePing,
		mode:        mode,
		spinner:     s,
		progressBar: prog,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, startPhase1Cmd(m.mode))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := strings.ToLower(msg.String())
		if key == "q" || key == "ctrl+c" || key == "esc" {
			m.quitting = true
			return m, tea.Quit
		}
		if m.state == stateResult {
			switch key {
			case "r", "enter":
				nm := initialModel(m.mode)
				return nm, nm.Init()
			case "m":
				nextM := nextBypassMode(m.mode)
				nm := initialModel(nextM)
				return nm, nm.Init()
			}
		}

	case phase1DoneMsg:
		m.meta = msg.meta
		m.nodes = msg.nodes
		m.bestPing = msg.bestPing
		m.state = stateDownload
		m.liveTelemetry = network.TransferTelemetry{}
		return m, startDownloadCmd(m.mode)

	case telemetryMsg:
		if msg.phase == m.state {
			m.liveTelemetry = msg.telemetry
		}
		return m, nil

	case downDoneMsg:
		m.downRes = msg.res
		if msg.res.BypassMethod != "" {
			m.meta.BypassMethod = msg.res.BypassMethod
		}
		m.state = stateUpload
		m.liveTelemetry = network.TransferTelemetry{}
		return m, startUploadCmd(m.mode)

	case upDoneMsg:
		m.upRes = msg.res
		m.state = stateResult
		if runOnce {
			return m, tea.Quit
		}
		return m, nil

	case spinner.TickMsg:
		if m.state != stateResult {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func renderMetadataPanel(meta network.ConnectionMeta, mode network.BypassMode) string {
	edge := meta.LocationSummary()
	if meta.Colo == "" && meta.Country == "" {
		edge = "Discovering..."
	}
	asn := meta.ASN
	if asn == "" {
		asn = "--"
	}
	ip := meta.MaskedIP()
	route := meta.BypassMethod
	if route == "" {
		route = fmt.Sprintf("Mode: %s", strings.ToUpper(string(mode)))
	}

	line1 := fmt.Sprintf("%s %-26s  %s %-12s  %s %-18s",
		labelStyle.Render("EDGE POP :"), valStyle.Render(edge),
		labelStyle.Render("ASN :"), valStyle.Render(asn),
		labelStyle.Render("CLIENT IP :"), valStyle.Render(ip),
	)
	line2 := fmt.Sprintf("%s %s (%s)   %s %s",
		labelStyle.Render("TRANSPORT:"), cyanValStyle.Render(route),
		subtitleStyle.Render(meta.TLSVersion),
		labelStyle.Render("ROUTE MODE:"), valStyle.Render(strings.ToUpper(string(mode))),
	)
	return panelStyle.Render(line1 + "\n" + line2)
}

func renderNodesPanel(nodes []network.PingResult) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf("%-20s %-19s %-11s %-16s %-10s %-8s %s",
		"BACKBONE NODE", "ENDPOINT", "AVG RTT", "MIN / MAX", "JITTER", "LOSS", "STATUS")) + "\n")
	b.WriteString(subtitleStyle.Render(strings.Repeat("─", 88)) + "\n")

	for i, n := range nodes {
		ep := fmt.Sprintf("%s:%d", n.Endpoint, n.Port)
		if n.Success {
			rtt := fmt.Sprintf("%.1f ms", n.PingMs)
			minMax := fmt.Sprintf("%.1f / %.1f ms", n.MinMs, n.MaxMs)
			jit := fmt.Sprintf("%.2f ms", n.JitterMs)
			loss := fmt.Sprintf("%.0f%%", n.LossPercent)
			status := greenValStyle.Render("ONLINE")
			b.WriteString(fmt.Sprintf("%-20s %-19s %-11s %-16s %-10s %-8s %s",
				valStyle.Render(n.Name),
				subtitleStyle.Render(ep),
				greenValStyle.Render(rtt),
				subtitleStyle.Render(minMax),
				valStyle.Render(jit),
				subtitleStyle.Render(loss),
				status,
			))
		} else {
			b.WriteString(fmt.Sprintf("%-20s %-19s %-11s %-16s %-10s %-8s %s",
				valStyle.Render(n.Name),
				subtitleStyle.Render(ep),
				subtitleStyle.Render("--"),
				subtitleStyle.Render("-- / --"),
				subtitleStyle.Render("--"),
				redValStyle.Render("100%"),
				redValStyle.Render("FILTERED"),
			))
		}
		if i < len(nodes)-1 {
			b.WriteString("\n")
		}
	}
	return panelStyle.Render(b.String())
}

func (m model) View() string {
	if m.quitting && m.state != stateResult {
		return ""
	}

	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(fmt.Sprintf(" %s  %s\n",
		headerTitleStyle.Render("NETPROBE"),
		headerTagStyle.Render("ANTI-CENSORSHIP SPEEDTEST & NETWORK DIAGNOSTIC SUITE"),
	))
	out.WriteString(" " + subtitleStyle.Render("Encrypted DoH Resolution  •  Direct Anycast Routing  •  Multi-Stream TLS 1.3") + "\n\n")

	out.WriteString(renderMetadataPanel(m.meta, m.mode) + "\n")

	if len(m.nodes) > 0 {
		out.WriteString(renderNodesPanel(m.nodes) + "\n")
	}

	switch m.state {
	case statePing:
		statusBox := activePanelStyle.Render(fmt.Sprintf(
			"%s %s\n%s",
			m.spinner.View(),
			cyanValStyle.Render("Phase 1/3: Probing backbone nodes over TCP/443 & resolving DoH edge route..."),
			subtitleStyle.Render("Bypassing ICMP blocks via concurrent TCP SYN/ACK handshakes"),
		))
		out.WriteString(statusBox + "\n")

	case stateDownload:
		mb := float64(m.liveTelemetry.BytesTransferred) / (1024 * 1024)
		streams := m.liveTelemetry.ActiveStreams
		if streams == 0 {
			streams = 4
		}
		liveBox := activePanelStyle.Render(fmt.Sprintf(
			"%s %s\n\n"+
				"  %s %-14s   %s %-14s   %s %-10s   %s %dx TLS\n\n"+
				"  %s",
			m.spinner.View(),
			cyanValStyle.Render("Phase 2/3: Benchmarking Multi-Stream Download Throughput..."),
			labelStyle.Render("LIVE:"), cyanValStyle.Render(fmt.Sprintf("%.2f Mbps", m.liveTelemetry.CurrentMbps)),
			labelStyle.Render("PEAK:"), valStyle.Render(fmt.Sprintf("%.2f Mbps", m.liveTelemetry.PeakMbps)),
			labelStyle.Render("VOLUME:"), valStyle.Render(fmt.Sprintf("%.1f MB", mb)),
			labelStyle.Render("STREAMS:"), streams,
			m.progressBar.ViewAs(m.liveTelemetry.Progress),
		))
		out.WriteString(liveBox + "\n")

	case stateUpload:
		mb := float64(m.liveTelemetry.BytesTransferred) / (1024 * 1024)
		streams := m.liveTelemetry.ActiveStreams
		if streams == 0 {
			streams = 3
		}
		liveBox := activePanelStyle.Render(fmt.Sprintf(
			"%s %s   (Download: %s)\n\n"+
				"  %s %-14s   %s %-14s   %s %-10s   %s %dx TLS\n\n"+
				"  %s",
			m.spinner.View(),
			indigoValStyle.Render("Phase 3/3: Benchmarking Multi-Stream Upload Throughput..."),
			cyanValStyle.Render(fmt.Sprintf("%.2f Mbps", m.downRes.SustainedMbps)),
			labelStyle.Render("LIVE:"), indigoValStyle.Render(fmt.Sprintf("%.2f Mbps", m.liveTelemetry.CurrentMbps)),
			labelStyle.Render("PEAK:"), valStyle.Render(fmt.Sprintf("%.2f Mbps", m.liveTelemetry.PeakMbps)),
			labelStyle.Render("VOLUME:"), valStyle.Render(fmt.Sprintf("%.1f MB", mb)),
			labelStyle.Render("STREAMS:"), streams,
			m.progressBar.ViewAs(m.liveTelemetry.Progress),
		))
		out.WriteString(liveBox + "\n")

	case stateResult:
		grade, gradeDesc := network.GradeConnection(
			m.bestPing.PingMs,
			m.bestPing.JitterMs,
			m.bestPing.LossPercent,
			m.downRes.SustainedMbps,
			m.upRes.SustainedMbps,
		)

		gradeRendered := greenValStyle.Render(grade)
		if strings.HasPrefix(grade, "CONGESTED") || strings.HasPrefix(grade, "DEGRADED") {
			gradeRendered = amberValStyle.Render(grade)
		}

		downMB := float64(m.downRes.BytesTransferred) / (1024 * 1024)
		upMB := float64(m.upRes.BytesTransferred) / (1024 * 1024)

		summary := fmt.Sprintf(
			"%s\n"+
				"%s\n"+
				"  %-16s %-22s   %-16s %s\n"+
				"  %-16s %-22s   %-16s %s\n"+
				"  %-16s %-22s   %-16s %s\n"+
				"%s\n"+
				"  %s %s  —  %s",
			labelStyle.Render("THROUGHPUT & LINK QUALITY SUMMARY"),
			subtitleStyle.Render(strings.Repeat("─", 88)),
			labelStyle.Render("DOWNLOAD       :"), cyanValStyle.Render(fmt.Sprintf("%.2f Mbps", m.downRes.SustainedMbps)),
			labelStyle.Render("DOWNLOAD PEAK  :"), valStyle.Render(fmt.Sprintf("%.2f Mbps (%.1f MB / %dx)", m.downRes.PeakMbps, downMB, m.downRes.Streams)),
			labelStyle.Render("UPLOAD         :"), indigoValStyle.Render(fmt.Sprintf("%.2f Mbps", m.upRes.SustainedMbps)),
			labelStyle.Render("UPLOAD PEAK    :"), valStyle.Render(fmt.Sprintf("%.2f Mbps (%.1f MB / %dx)", m.upRes.PeakMbps, upMB, m.upRes.Streams)),
			labelStyle.Render("UNLOADED RTT   :"), greenValStyle.Render(fmt.Sprintf("%.1f ms", m.bestPing.PingMs)),
			labelStyle.Render("JITTER / LOSS  :"), valStyle.Render(fmt.Sprintf("%.2f ms  /  %.1f%% loss", m.bestPing.JitterMs, m.bestPing.LossPercent)),
			subtitleStyle.Render(strings.Repeat("─", 88)),
			labelStyle.Render("LINK GRADE     :"), gradeRendered,
			subtitleStyle.Render(gradeDesc),
		)
		out.WriteString(panelStyle.Render(summary) + "\n")

		if !runOnce && !m.quitting {
			controls := fmt.Sprintf(
				" %s %s    %s %s    %s %s",
				cyanValStyle.Render("[R / Enter]"), subtitleStyle.Render("Run Diagnostic Again"),
				indigoValStyle.Render("[M]"), subtitleStyle.Render(fmt.Sprintf("Switch Route Mode (Current: %s)", strings.ToUpper(string(m.mode)))),
				amberValStyle.Render("[Q / Esc]"), subtitleStyle.Render("Exit"),
			)
			out.WriteString("\n" + controls + "\n")
		}
	}

	return out.String()
}

var rootCmd = &cobra.Command{
	Use:   "speedtest-cli",
	Short: "NetProbe // Anti-Censorship Speedtest & Network Diagnostic CLI",
	Long:  "High-precision multi-stream speedtest and network diagnostic utility engineered with encrypted DoH resolution, direct Anycast routing, and TCP/443 probing to bypass standard firewall and DNS restrictions.",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			runJSONMode()
			return
		}

		mode := parseBypassMode(bypassFlag)
		program = tea.NewProgram(initialModel(mode))
		if _, err := program.Run(); err != nil {
			fmt.Printf("Error running diagnostic interface: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output diagnostic report in JSON format")
	rootCmd.Flags().BoolVar(&runOnce, "once", false, "Run diagnostic once and exit immediately")
	rootCmd.Flags().StringVarP(&bypassFlag, "mode", "m", "auto", "Routing bypass mode: auto, doh, direct, system")
	rootCmd.Flags().IntVarP(&streamCount, "streams", "s", 0, "Number of concurrent TLS transfer streams (default: 4 down / 3 up)")
	rootCmd.Flags().IntVarP(&durationSec, "duration", "d", 0, "Duration in seconds per bandwidth phase (default: 7s down / 6s up)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
