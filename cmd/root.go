package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"speedtest-cli/internal/network"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	program    *tea.Program

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FFAA")).
			MarginBottom(1)

	resultStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#888888")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	serverStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00AAFF")).
			Bold(true)

	metricStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true).
			MarginTop(1)

	stageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Bold(true)
)

type JSONResult struct {
	PingMs       int64   `json:"ping_ms"`
	JitterMs     int64   `json:"jitter_ms"`
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
}

func runJSONMode() {
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

	downSpeed := network.TestDownload(nil)
	upSpeed := network.TestUpload(nil)

	result := JSONResult{
		PingMs:       finalPing,
		JitterMs:     finalJitter,
		DownloadMbps: downSpeed,
		UploadMbps:   upSpeed,
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

type pingResultMsg struct {
	results string
}

type downResultMsg struct {
	speed float64
}

type upResultMsg struct {
	speed float64
}

type progressMsg float64

func startPing() tea.Msg {
	endpoints := []string{"1.1.1.1", "8.8.8.8"}
	resultText := ""

	for _, ep := range endpoints {
		pingRes := network.MeasurePingAndJitter(ep)
		if pingRes.Success {
			resultText += fmt.Sprintf("%s\nPing: %s | Jitter: %s\n\n",
				serverStyle.Render(fmt.Sprintf("Server: %s", pingRes.Endpoint)),
				metricStyle.Render(fmt.Sprintf("%d ms", pingRes.Ping.Milliseconds())),
				metricStyle.Render(fmt.Sprintf("%d ms", pingRes.Jitter.Milliseconds())),
			)
		} else {
			resultText += errorStyle.Render(fmt.Sprintf("Failed to reach %s\n\n", ep))
		}
	}
	return pingResultMsg{results: resultText}
}

func startDownload() tea.Msg {
	speed := network.TestDownload(func(percent float64) {
		if program != nil {
			program.Send(progressMsg(percent))
		}
	})
	return downResultMsg{speed: speed}
}

func startUpload() tea.Msg {
	speed := network.TestUpload(func(percent float64) {
		if program != nil {
			program.Send(progressMsg(percent))
		}
	})
	return upResultMsg{speed: speed}
}

type model struct {
	state       state
	spinner     spinner.Model
	progressBar progress.Model
	form        *huh.Form

	pingResults string
	downSpeed   float64
	upSpeed     float64

	quitting bool
	progress float64
	action   string
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 40

	return model{
		state:       statePing,
		spinner:     s,
		progressBar: prog,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, startPing)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

	case pingResultMsg:
		m.pingResults = msg.results
		m.state = stateDownload
		m.progress = 0
		return m, startDownload

	case progressMsg:
		m.progress = float64(msg)
		return m, nil

	case downResultMsg:
		m.downSpeed = msg.speed
		m.state = stateUpload
		m.progress = 0
		return m, startUpload

	case upResultMsg:
		m.upSpeed = msg.speed
		m.state = stateResult

		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Diagnostic complete. What next?").
					Options(
						huh.NewOption("🔄 Run Test Again", "restart"),
						huh.NewOption("❌ Exit", "exit"),
					).
					Value(&m.action),
			),
		)
		return m, m.form.Init()

	case spinner.TickMsg:
		if m.state == statePing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	if m.state == stateResult && m.form != nil {
		formModel, cmd := m.form.Update(msg)
		if f, ok := formModel.(*huh.Form); ok {
			m.form = f
			if m.form.State == huh.StateCompleted {
				if m.action == "restart" {
					newModel := initialModel()
					return newModel, tea.Batch(tea.ClearScreen, newModel.Init())
				}
				m.quitting = true
				return m, tea.Quit
			}
		}
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	out := titleStyle.Render("🚀 ANTI-CENSORSHIP SPEEDTEST CLI") + "\n\n"

	switch m.state {
	case statePing:
		out += fmt.Sprintf("%s %s\n", m.spinner.View(), stageStyle.Render("Pinging reliable endpoints..."))
	case stateDownload:
		out += m.pingResults
		out += fmt.Sprintf("\n%s\n", stageStyle.Render("Downloading 50MB dummy payload..."))
		out += m.progressBar.ViewAs(m.progress) + "\n"
	case stateUpload:
		out += m.pingResults
		out += fmt.Sprintf("\n%s\n", stageStyle.Render("Uploading 10MB dummy payload..."))
		out += m.progressBar.ViewAs(m.progress) + "\n"
	case stateResult:
		resultText := m.pingResults
		resultText += fmt.Sprintf("%s\nDownload: %s | Upload: %s",
			serverStyle.Render("Bandwidth (Cloudflare CDN)"),
			metricStyle.Render(fmt.Sprintf("%.2f Mbps", m.downSpeed)),
			metricStyle.Render(fmt.Sprintf("%.2f Mbps", m.upSpeed)),
		)
		out += resultStyle.Render(resultText) + "\n\n"

		if m.form != nil {
			out += m.form.View()
		}
	}

	return out
}

var rootCmd = &cobra.Command{
	Use:   "speedtest-cli",
	Short: "Anti-censorship Speedtest CLI",
	Long:  "A Network Diagnostic and Speedtest CLI tool designed to bypass standard firewall/DNS blocks.",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			runJSONMode()
			return
		}

		program = tea.NewProgram(initialModel())
		if _, err := program.Run(); err != nil {
			fmt.Printf("Alas, there's been an error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output results in JSON format")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
