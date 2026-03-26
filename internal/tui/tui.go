package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yusufkaran/porttracker/internal/process"
	"github.com/yusufkaran/porttracker/internal/scanner"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			PaddingRight(1)

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("25")).
			Foreground(lipgloss.Color("255"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	portStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	projectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246"))

	uptimeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			MarginTop(1)

	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			MarginTop(1)
)

type model struct {
	ports    []process.PortInfo
	cursor   int
	width    int
	height   int
	status   string
	err      error
	quitting bool
}

type portsMsg []process.PortInfo
type errMsg error
type killMsg struct{ port int }

func initialModel() model {
	return model{}
}

func (m model) Init() tea.Cmd {
	return scanPorts
}

func scanPorts() tea.Msg {
	ports, err := scanner.Scan()
	if err != nil {
		return errMsg(err)
	}
	return portsMsg(ports)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case portsMsg:
		m.ports = msg
		m.err = nil
		if m.cursor >= len(m.ports) {
			m.cursor = max(0, len(m.ports)-1)
		}

	case errMsg:
		m.err = msg

	case killMsg:
		m.status = fmt.Sprintf("Killed process on port %d", msg.port)
		return m, scanPorts

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.ports)-1 {
				m.cursor++
			}

		case key.Matches(msg, keys.Kill):
			if len(m.ports) > 0 && m.cursor < len(m.ports) {
				p := m.ports[m.cursor]
				err := scanner.KillPID(p.PID)
				if err != nil {
					m.status = fmt.Sprintf("Error killing port %d: %s", p.Port, err)
				} else {
					return m, func() tea.Msg {
						return killMsg{port: p.Port}
					}
				}
			}

		case key.Matches(msg, keys.KillAll):
			if len(m.ports) > 0 && m.cursor < len(m.ports) {
				p := m.ports[m.cursor]
				killed := 0
				for _, port := range m.ports {
					if port.Project == p.Project && port.Dir == p.Dir {
						if err := scanner.KillPID(port.PID); err == nil {
							killed++
						}
					}
				}
				m.status = fmt.Sprintf("Killed %d processes for project %s", killed, p.Project)
				return m, scanPorts
			}

		case key.Matches(msg, keys.Open):
			if len(m.ports) > 0 && m.cursor < len(m.ports) {
				p := m.ports[m.cursor]
				url := fmt.Sprintf("http://localhost:%d", p.Port)
				openBrowser(url)
				m.status = fmt.Sprintf("Opened %s", url)
			}

		case key.Matches(msg, keys.Refresh):
			m.status = "Refreshing..."
			return m, scanPorts
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ PortTracker"))
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		b.WriteString("\n")
	}

	if len(m.ports) == 0 {
		b.WriteString(emptyStyle.Render("No listening ports found."))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("r refresh • q quit"))
		return b.String()
	}

	// Column widths
	colPort := 7
	colPID := 8
	colProject := 20
	colDir := 30
	colCommand := 18
	colUptime := 10

	// Adjust dir column based on terminal width
	if m.width > 0 {
		used := colPort + colPID + colProject + colCommand + colUptime + 6 // 6 for separators
		remaining := m.width - used
		if remaining > 10 {
			colDir = remaining
		}
	}

	// Header
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s",
		colPort, "PORT",
		colPID, "PID",
		colProject, "PROJECT",
		colDir, "DIRECTORY",
		colCommand, "COMMAND",
		colUptime, "UPTIME",
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Rows
	for i, p := range m.ports {
		dir := truncateLeft(p.Dir, colDir)
		project := truncate(p.Project, colProject)
		cmd := truncate(p.Command, colCommand)

		row := fmt.Sprintf("%-*s %-*d %-*s %-*s %-*s %-*s",
			colPort, portStyle.Render(strconv.Itoa(p.Port)),
			colPID, p.PID,
			colProject, projectStyle.Render(project),
			colDir, dir,
			colCommand, commandStyle.Render(cmd),
			colUptime, uptimeStyle.Render(p.UptimeStr()),
		)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")
	}

	if m.status != "" {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("↑↓ navigate • k kill • K kill project • o open • r refresh • q quit"))

	return b.String()
}

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Kill    key.Binding
	KillAll key.Binding
	Open    key.Binding
	Refresh key.Binding
	Quit    key.Binding
}

var keys = keyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k")),
	Down:    key.NewBinding(key.WithKeys("down", "j")),
	Kill:    key.NewBinding(key.WithKeys("x", "delete")),
	KillAll: key.NewBinding(key.WithKeys("X")),
	Open:    key.NewBinding(key.WithKeys("o")),
	Refresh: key.NewBinding(key.WithKeys("r")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c")),
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func truncateLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[len(s)-maxLen:]
	}
	return "..." + s[len(s)-maxLen+3:]
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	}
}

func Run() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
