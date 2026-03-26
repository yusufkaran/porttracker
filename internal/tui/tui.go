package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
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

	groupHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("75"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	searchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	confirmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmKill
	confirmKillAll
)

type model struct {
	allPorts []process.PortInfo // unfiltered
	ports    []process.PortInfo // displayed (filtered + sorted)
	cursor   int
	width    int
	height   int
	status   string
	err      error
	quitting bool
	// search
	searching bool
	query     string
	// confirm
	confirming    confirmAction
	confirmTarget process.PortInfo
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

// sortAndGroup sorts ports: projects with directories first (grouped), system ports last
func sortAndGroup(ports []process.PortInfo) []process.PortInfo {
	sorted := make([]process.PortInfo, len(ports))
	copy(sorted, ports)

	sort.SliceStable(sorted, func(i, j int) bool {
		iHasDir := sorted[i].Dir != "" && sorted[i].Dir != "/"
		jHasDir := sorted[j].Dir != "" && sorted[j].Dir != "/"

		// Ports with directories come first
		if iHasDir != jHasDir {
			return iHasDir
		}

		// Within same group, sort by project name then port
		if iHasDir && jHasDir {
			if sorted[i].Project != sorted[j].Project {
				return sorted[i].Project < sorted[j].Project
			}
		}
		return sorted[i].Port < sorted[j].Port
	})

	return sorted
}

func filterPorts(ports []process.PortInfo, query string) []process.PortInfo {
	if query == "" {
		return ports
	}
	q := strings.ToLower(query)
	var result []process.PortInfo
	for _, p := range ports {
		if strings.Contains(strings.ToLower(p.Project), q) ||
			strings.Contains(strings.ToLower(p.Dir), q) ||
			strings.Contains(strings.ToLower(p.Command), q) ||
			strings.Contains(strconv.Itoa(p.Port), q) {
			result = append(result, p)
		}
	}
	return result
}

func (m *model) updateFiltered() {
	m.ports = sortAndGroup(filterPorts(m.allPorts, m.query))
	if m.cursor >= len(m.ports) {
		m.cursor = max(0, len(m.ports)-1)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case portsMsg:
		m.allPorts = msg
		m.err = nil
		m.status = ""
		m.updateFiltered()

	case errMsg:
		m.err = msg

	case killMsg:
		m.status = fmt.Sprintf("Killed process on port %d", msg.port)
		return m, scanPorts

	case tea.KeyMsg:
		// Search mode input
		if m.searching {
			switch msg.String() {
			case "enter", "esc":
				m.searching = false
				if msg.String() == "esc" {
					m.query = ""
					m.updateFiltered()
				}
			case "backspace":
				if len(m.query) > 0 {
					m.query = m.query[:len(m.query)-1]
					m.updateFiltered()
				}
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				if len(msg.String()) == 1 {
					m.query += msg.String()
					m.updateFiltered()
				}
			}
			return m, nil
		}

		// Confirm mode
		if m.confirming != confirmNone {
			switch msg.String() {
			case "y", "Y":
				if m.confirming == confirmKill {
					p := m.confirmTarget
					err := scanner.KillPID(p.PID)
					m.confirming = confirmNone
					if err != nil {
						m.status = fmt.Sprintf("Error killing port %d: %s", p.Port, err)
					} else {
						return m, func() tea.Msg {
							return killMsg{port: p.Port}
						}
					}
				} else if m.confirming == confirmKillAll {
					p := m.confirmTarget
					killed := 0
					for _, port := range m.ports {
						if port.Project == p.Project && port.Dir == p.Dir {
							if err := scanner.KillPID(port.PID); err == nil {
								killed++
							}
						}
					}
					m.confirming = confirmNone
					m.status = fmt.Sprintf("Killed %d processes for project %s", killed, p.Project)
					return m, scanPorts
				}
			default:
				m.confirming = confirmNone
				m.status = ""
			}
			return m, nil
		}

		// Normal mode
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Search):
			m.searching = true
			m.query = ""
			return m, nil

		case key.Matches(msg, keys.ClearSearch):
			m.query = ""
			m.updateFiltered()

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
				m.confirming = confirmKill
				m.confirmTarget = p
				m.status = fmt.Sprintf("Kill port %d (%s)? [y/N]", p.Port, p.Command)
			}

		case key.Matches(msg, keys.KillAll):
			if len(m.ports) > 0 && m.cursor < len(m.ports) {
				p := m.ports[m.cursor]
				count := 0
				for _, port := range m.ports {
					if port.Project == p.Project && port.Dir == p.Dir {
						count++
					}
				}
				m.confirming = confirmKillAll
				m.confirmTarget = p
				m.status = fmt.Sprintf("Kill all %d ports for %s? [y/N]", count, p.Project)
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

	// Search bar
	if m.searching {
		b.WriteString(searchStyle.Render("/ ") + m.query + "█")
		b.WriteString("\n")
	} else if m.query != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("filter: %s  ", m.query)) + dimStyle.Render("(esc clear)"))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		b.WriteString("\n")
	}

	if len(m.ports) == 0 {
		if m.query != "" {
			b.WriteString(emptyStyle.Render(fmt.Sprintf("No ports matching '%s'", m.query)))
		} else {
			b.WriteString(emptyStyle.Render("No listening ports found."))
		}
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

	if m.width > 0 {
		used := colPort + colPID + colProject + colCommand + colUptime + 6
		remaining := m.width - used
		if remaining > 10 {
			colDir = remaining
		}
	}

	// Header
	header := pad("PORT", colPort) + pad("PID", colPID) + pad("PROJECT", colProject) +
		pad("DIRECTORY", colDir) + pad("COMMAND", colCommand) + pad("UPTIME", colUptime)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	// Rows with group headers
	lastProject := ""
	lastDir := ""
	rowIdx := 0
	for _, p := range m.ports {
		hasDir := p.Dir != "" && p.Dir != "/"
		groupKey := p.Project + "|" + p.Dir

		// Show group separator when project changes (only for project ports)
		if hasDir && groupKey != lastProject+"|"+lastDir {
			if rowIdx > 0 {
				b.WriteString("\n")
			}
			label := fmt.Sprintf("── %s ", p.Project)
			if p.Dir != "" {
				label += dimStyle.Render(fmt.Sprintf("(%s)", shortenDir(p.Dir)))
			}
			b.WriteString(groupHeaderStyle.Render(label))
			b.WriteString("\n")
		} else if !hasDir && lastDir != "" {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("── System / Other"))
			b.WriteString("\n")
			lastDir = ""
		}

		if hasDir {
			lastProject = p.Project
			lastDir = p.Dir
		}

		dir := truncateLeft(p.Dir, colDir)
		project := truncate(p.Project, colProject)
		cmd := truncate(p.Command, colCommand)

		portPad := pad(strconv.Itoa(p.Port), colPort)
		pidPad := pad(strconv.Itoa(p.PID), colPID)
		projPad := pad(project, colProject)
		dirPad := pad(dir, colDir)
		cmdPad := pad(cmd, colCommand)
		uptPad := pad(p.UptimeStr(), colUptime)

		var row string
		if rowIdx == m.cursor {
			row = selectedStyle.Render(portPad + pidPad + projPad + dirPad + cmdPad + uptPad)
		} else if !hasDir {
			row = systemStyle.Render(portPad+pidPad+projPad+dirPad+cmdPad+uptPad)
		} else {
			row = portStyle.Render(portPad) +
				normalStyle.Render(pidPad) +
				projectStyle.Render(projPad) +
				normalStyle.Render(dirPad) +
				commandStyle.Render(cmdPad) +
				uptimeStyle.Render(uptPad)
		}

		b.WriteString(row)
		b.WriteString("\n")
		rowIdx++
	}

	if m.status != "" {
		if m.confirming != confirmNone {
			b.WriteString(confirmStyle.Render(m.status))
		} else {
			b.WriteString(statusStyle.Render(m.status))
		}
		b.WriteString("\n")
	}

	if m.confirming != confirmNone {
		b.WriteString(helpStyle.Render("y confirm • any key cancel"))
	} else if m.searching {
		b.WriteString(helpStyle.Render("enter confirm • esc cancel"))
	} else {
		b.WriteString(helpStyle.Render("↑↓/jk navigate • x kill • X kill project • o open • / search • r refresh • q quit"))
	}

	return b.String()
}

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	Kill        key.Binding
	KillAll     key.Binding
	Open        key.Binding
	Refresh     key.Binding
	Quit        key.Binding
	Search      key.Binding
	ClearSearch key.Binding
}

var keys = keyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k")),
	Down:        key.NewBinding(key.WithKeys("down", "j")),
	Kill:        key.NewBinding(key.WithKeys("x", "backspace", "delete")),
	KillAll:     key.NewBinding(key.WithKeys("X")),
	Open:        key.NewBinding(key.WithKeys("o")),
	Refresh:     key.NewBinding(key.WithKeys("r")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Search:      key.NewBinding(key.WithKeys("/")),
	ClearSearch: key.NewBinding(key.WithKeys("esc")),
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-len(s))
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

func shortenDir(dir string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(dir, home) {
		return "~" + dir[len(home):]
	}
	return dir
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
