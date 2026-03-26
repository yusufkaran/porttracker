package process

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PortInfo struct {
	Port    int
	PID     int
	Command string
	Dir     string
	Project string
	Uptime  time.Duration
	User    string
}

func (p PortInfo) UptimeStr() string {
	d := p.Uptime
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 24 {
		days := h / 24
		h = h % 24
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func DetectProject(dir string) string {
	if dir == "" {
		return "-"
	}

	// Check package.json
	pkgJSON := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		var pkg struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Name != "" {
			return pkg.Name
		}
	}

	// Check go.mod
	goMod := filepath.Join(dir, "go.mod")
	if data, err := os.ReadFile(goMod); err == nil {
		lines := strings.SplitN(string(data), "\n", 2)
		if len(lines) > 0 {
			mod := strings.TrimPrefix(lines[0], "module ")
			mod = strings.TrimSpace(mod)
			if idx := strings.LastIndex(mod, "/"); idx >= 0 {
				mod = mod[idx+1:]
			}
			if mod != "" {
				return mod
			}
		}
	}

	// Check Cargo.toml
	cargoToml := filepath.Join(dir, "Cargo.toml")
	if data, err := os.ReadFile(cargoToml); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					name := strings.Trim(strings.TrimSpace(parts[1]), "\"")
					if name != "" {
						return name
					}
				}
			}
		}
	}

	// Check pyproject.toml
	pyproject := filepath.Join(dir, "pyproject.toml")
	if data, err := os.ReadFile(pyproject); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					name := strings.Trim(strings.TrimSpace(parts[1]), "\"")
					if name != "" {
						return name
					}
				}
			}
		}
	}

	// Fallback: directory name
	return filepath.Base(dir)
}
