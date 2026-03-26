package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/yusufkaran/porttracker/internal/process"
	"github.com/yusufkaran/porttracker/internal/scanner"
	"github.com/yusufkaran/porttracker/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "ls", "list":
		cmdList()
	case "kill":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: pt kill <port|project-name>")
			os.Exit(1)
		}
		cmdKill(os.Args[2])
	case "version", "--version", "-v":
		fmt.Printf("pt version %s\n", version)
	case "help", "--help", "-h":
		printHelp()
	default:
		// Try as port number for quick kill
		if port, err := strconv.Atoi(os.Args[1]); err == nil {
			cmdKillPort(port)
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

func cmdList() {
	ports, err := scanner.Scan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if len(ports) == 0 {
		fmt.Println("No listening ports found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tPID\tPROJECT\tDIRECTORY\tCOMMAND\tUPTIME")
	for _, p := range ports {
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%s\n",
			p.Port, p.PID, p.Project, shortenDir(p.Dir), p.Command, p.UptimeStr())
	}
	w.Flush()
}

func cmdKill(target string) {
	// Try as port number first
	if port, err := strconv.Atoi(target); err == nil {
		cmdKillPort(port)
		return
	}

	// Try as project name
	cmdKillProject(target)
}

func cmdKillPort(port int) {
	if err := scanner.KillPort(port); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Killed process on port %d\n", port)
}

func cmdKillProject(name string) {
	ports, err := scanner.Scan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	killed := 0
	for _, p := range ports {
		if matchProject(p, name) {
			if err := scanner.KillPID(p.PID); err == nil {
				fmt.Printf("Killed port %d (PID %d) — %s\n", p.Port, p.PID, p.Project)
				killed++
			}
		}
	}

	if killed == 0 {
		fmt.Fprintf(os.Stderr, "No processes found matching '%s'\n", name)
		os.Exit(1)
	}
}

func matchProject(p process.PortInfo, name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(strings.ToLower(p.Project), name) ||
		strings.Contains(strings.ToLower(p.Dir), name) ||
		strings.Contains(strings.ToLower(p.Command), name)
}

func shortenDir(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	if strings.HasPrefix(dir, home) {
		return "~" + dir[len(home):]
	}
	return dir
}

func printHelp() {
	fmt.Println(`PortTracker — manage your localhost ports

Usage:
  pt              Interactive TUI
  pt ls           List all listening ports
  pt kill <port>  Kill process by port number
  pt kill <name>  Kill processes by project name
  pt version      Show version

Shortcuts:
  pt 3000         Same as: pt kill 3000`)
}
