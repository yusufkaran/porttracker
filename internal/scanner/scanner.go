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

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
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

	// Deduplicate by port
	seen := make(map[int]bool)
	var unique []rawEntry
	for _, e := range entries {
		if !seen[e.port] {
			seen[e.port] = true
			unique = append(unique, e)
		}
	}

	// Collect unique PIDs
	pidSet := make(map[int]bool)
	for _, e := range unique {
		pidSet[e.pid] = true
	}
	var pids []int
	for pid := range pidSet {
		pids = append(pids, pid)
	}

	// Batch fetch: dirs, uptimes, users in single calls
	dirs := batchGetDirs(pids)
	uptimes, users := batchGetPsInfo(pids)

	var results []process.PortInfo
	for _, e := range unique {
		dir := dirs[e.pid]
		info := process.PortInfo{
			Port:    e.port,
			PID:     e.pid,
			Command: e.command,
			Dir:     dir,
			Project: process.DetectProject(dir),
			Uptime:  uptimes[e.pid],
			User:    users[e.pid],
		}
		results = append(results, info)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})

	return results, nil
}

// batchGetDirs gets working directories for all PIDs in one lsof call
func batchGetDirs(pids []int) map[int]string {
	dirs := make(map[int]string)
	if len(pids) == 0 {
		return dirs
	}

	switch runtime.GOOS {
	case "darwin":
		// Build comma-separated PID list for lsof
		pidStrs := make([]string, len(pids))
		for i, pid := range pids {
			pidStrs[i] = strconv.Itoa(pid)
		}
		pidArg := strings.Join(pidStrs, ",")

		out, err := exec.Command("lsof", "-a", "-p", pidArg, "-d", "cwd", "-Fpn").Output()
		if err != nil {
			return dirs
		}

		var currentPID int
		for _, line := range strings.Split(string(out), "\n") {
			if len(line) < 2 {
				continue
			}
			switch line[0] {
			case 'p':
				pid, _ := strconv.Atoi(line[1:])
				currentPID = pid
			case 'n':
				dir := line[1:]
				if dir != "" && dir != "/" {
					dirs[currentPID] = dir
				}
			}
		}

	case "linux":
		for _, pid := range pids {
			link, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
			if err == nil {
				dirs[pid] = link
			}
		}
	}

	return dirs
}

// batchGetPsInfo gets uptime and user for all PIDs in one ps call
func batchGetPsInfo(pids []int) (map[int]time.Duration, map[int]string) {
	uptimes := make(map[int]time.Duration)
	users := make(map[int]string)
	if len(pids) == 0 {
		return uptimes, users
	}

	pidStrs := make([]string, len(pids))
	for i, pid := range pids {
		pidStrs[i] = strconv.Itoa(pid)
	}
	pidArg := strings.Join(pidStrs, ",")

	out, err := exec.Command("ps", "-o", "pid=,etime=,user=", "-p", pidArg).Output()
	if err != nil {
		return uptimes, users
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		uptimes[pid] = parseEtime(fields[1])
		users[pid] = fields[2]
	}

	return uptimes, users
}

// parseEtime parses ps etime format: [[dd-]hh:]mm:ss
func parseEtime(s string) time.Duration {
	var days, hours, minutes, seconds int

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
