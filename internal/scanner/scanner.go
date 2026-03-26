package scanner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yusufkaran/porttracker/internal/process"
)

func Scan() ([]process.PortInfo, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return scanUnix()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func scanUnix() ([]process.PortInfo, error) {
	out, err := exec.Command("lsof", "-iTCP", "-sTCP:LISTEN", "-nP", "-F", "pcnf").Output()
	if err != nil {
		// lsof may exit 1 if no results
		if len(out) == 0 {
			return nil, nil
		}
	}

	type rawEntry struct {
		pid     int
		command string
		port    int
	}

	var entries []rawEntry
	var currentPID int
	var currentCommand string

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}
		field := line[0]
		value := line[1:]

		switch field {
		case 'p':
			pid, _ := strconv.Atoi(value)
			currentPID = pid
		case 'c':
			currentCommand = value
		case 'n':
			// Format: *:PORT or 127.0.0.1:PORT or [::1]:PORT
			idx := strings.LastIndex(value, ":")
			if idx >= 0 {
				portStr := value[idx+1:]
				port, err := strconv.Atoi(portStr)
				if err == nil && port > 0 {
					entries = append(entries, rawEntry{
						pid:     currentPID,
						command: currentCommand,
						port:    port,
					})
				}
			}
		}
	}

	// Deduplicate by port (same port can appear for IPv4 and IPv6)
	seen := make(map[int]bool)
	var unique []rawEntry
	for _, e := range entries {
		if !seen[e.port] {
			seen[e.port] = true
			unique = append(unique, e)
		}
	}

	var results []process.PortInfo
	for _, e := range unique {
		dir := getProcessDir(e.pid)
		info := process.PortInfo{
			Port:    e.port,
			PID:     e.pid,
			Command: e.command,
			Dir:     dir,
			Project: process.DetectProject(dir),
			Uptime:  getProcessUptime(e.pid),
			User:    getProcessUser(e.pid),
		}
		results = append(results, info)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})

	return results, nil
}

func getProcessDir(pid int) string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") && !strings.HasPrefix(line, "n/dev") {
				return line[1:]
			}
		}
	case "linux":
		link, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
		if err == nil {
			return link
		}
	}
	return ""
}

func getProcessUptime(pid int) time.Duration {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ps", "-o", "etime=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return 0
		}
		return parseEtime(strings.TrimSpace(string(out)))
	case "linux":
		out, err := exec.Command("ps", "-o", "etimes=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return 0
		}
		secs, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return time.Duration(secs) * time.Second
	}
	return 0
}

// parseEtime parses ps etime format: [[dd-]hh:]mm:ss
func parseEtime(s string) time.Duration {
	var days, hours, minutes, seconds int

	// Check for days
	if idx := strings.Index(s, "-"); idx >= 0 {
		days, _ = strconv.Atoi(s[:idx])
		s = s[idx+1:]
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
		hours, _ = strconv.Atoi(parts[0])
		minutes, _ = strconv.Atoi(parts[1])
		seconds, _ = strconv.Atoi(parts[2])
	case 2:
		minutes, _ = strconv.Atoi(parts[0])
		seconds, _ = strconv.Atoi(parts[1])
	case 1:
		seconds, _ = strconv.Atoi(parts[0])
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

func getProcessUser(pid int) string {
	out, err := exec.Command("ps", "-o", "user=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func KillPort(port int) error {
	ports, err := Scan()
	if err != nil {
		return err
	}
	for _, p := range ports {
		if p.Port == port {
			proc, err := os.FindProcess(p.PID)
			if err != nil {
				return fmt.Errorf("process %d not found: %w", p.PID, err)
			}
			return proc.Signal(os.Kill)
		}
	}
	return fmt.Errorf("no process found on port %d", port)
}

func KillPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	return proc.Signal(os.Kill)
}
