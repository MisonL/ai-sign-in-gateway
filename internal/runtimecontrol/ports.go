package runtimecontrol

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type PortProcess struct {
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

type StopPortResult struct {
	Port    int    `json:"port"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Stopped bool   `json:"stopped"`
	Skipped bool   `json:"skipped"`
	Message string `json:"message"`
}

func DescribePortOccupant(port int) string {
	processes, err := ProcessesListeningOnPort(port)
	if err != nil || len(processes) == 0 {
		return "未知进程"
	}
	process := processes[0]
	if process.Command == "" {
		return fmt.Sprintf("pid:%d", process.PID)
	}
	return fmt.Sprintf("%s(pid:%d)", shortCommand(process.Command), process.PID)
}

func StopAppProcessesOnPorts(ports []int, currentPorts map[int]bool, appName string) []StopPortResult {
	results := make([]StopPortResult, 0, len(ports))
	seen := map[int]bool{}
	for _, port := range ports {
		if port <= 0 || seen[port] {
			continue
		}
		seen[port] = true
		if currentPorts[port] {
			results = append(results, StopPortResult{
				Port:    port,
				Skipped: true,
				Message: "当前程序正在使用该端口",
			})
			continue
		}
		processes, err := ProcessesListeningOnPort(port)
		if err != nil {
			results = append(results, StopPortResult{
				Port:    port,
				Skipped: true,
				Message: "无法读取端口占用: " + err.Error(),
			})
			continue
		}
		if len(processes) == 0 {
			results = append(results, StopPortResult{
				Port:    port,
				Skipped: true,
				Message: "端口未占用",
			})
			continue
		}
		for _, process := range processes {
			result := StopPortResult{
				Port:    port,
				PID:     process.PID,
				Command: process.Command,
			}
			if process.PID == os.Getpid() {
				result.Skipped = true
				result.Message = "跳过当前进程"
				results = append(results, result)
				continue
			}
			if !IsAppProcess(process, appName) {
				result.Skipped = true
				result.Message = "不是本程序进程，已跳过"
				results = append(results, result)
				continue
			}
			if err := terminateProcess(process.PID, port); err != nil {
				result.Message = "停止失败: " + err.Error()
			} else {
				result.Stopped = true
				result.Message = "已停止"
			}
			results = append(results, result)
		}
	}
	return results
}

func ProcessesListeningOnPort(port int) ([]PortProcess, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsProcessesListeningOnPort(port)
	default:
		return unixProcessesListeningOnPort(port)
	}
}

func IsAppProcess(process PortProcess, appName string) bool {
	command := strings.ToLower(process.Command)
	app := strings.ToLower(strings.TrimSpace(appName))
	return strings.Contains(command, app) || strings.Contains(command, "ai-sign-i")
}

func IsPortOccupied(host string, port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = listener.Close()
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") || strings.Contains(msg, "only one usage of each socket address")
}

func unixProcessesListeningOnPort(port int) ([]PortProcess, error) {
	if output, err := commandOutput("lsof", "-nP", "-tiTCP:"+strconv.Itoa(port), "-sTCP:LISTEN"); err == nil {
		pids := parsePIDs(output)
		processes := make([]PortProcess, 0, len(pids))
		for _, pid := range pids {
			processes = append(processes, PortProcess{
				Port:    port,
				PID:     pid,
				Command: processCommand(pid),
			})
		}
		return processes, nil
	}
	if output, err := commandOutput("ss", "-ltnp", "sport = :"+strconv.Itoa(port)); err == nil {
		return parseSSProcesses(port, output), nil
	}
	return nil, nil
}

func windowsProcessesListeningOnPort(port int) ([]PortProcess, error) {
	output, err := commandOutput("netstat", "-ano", "-p", "tcp")
	if err != nil {
		return nil, err
	}
	target := ":" + strconv.Itoa(port)
	var processes []PortProcess
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[3], "LISTENING") || !strings.HasSuffix(fields[1], target) {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		processes = append(processes, PortProcess{
			Port:    port,
			PID:     pid,
			Command: windowsProcessCommand(pid),
		})
	}
	return processes, nil
}

func parsePIDs(output string) []int {
	items := []int{}
	seen := map[int]bool{}
	for _, field := range strings.Fields(output) {
		pid, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		items = append(items, pid)
	}
	return items
}

func parseSSProcesses(port int, output string) []PortProcess {
	var processes []PortProcess
	seen := map[int]bool{}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "LISTEN") || !strings.Contains(line, "pid=") {
			continue
		}
		parts := strings.Split(line, "pid=")
		for _, part := range parts[1:] {
			pidText := part
			if idx := strings.IndexAny(pidText, ",)"); idx >= 0 {
				pidText = pidText[:idx]
			}
			pid, err := strconv.Atoi(strings.TrimSpace(pidText))
			if err != nil || pid <= 0 || seen[pid] {
				continue
			}
			seen[pid] = true
			processes = append(processes, PortProcess{
				Port:    port,
				PID:     pid,
				Command: processCommand(pid),
			})
		}
	}
	return processes
}

func processCommand(pid int) string {
	if output, err := commandOutput("ps", "-p", strconv.Itoa(pid), "-o", "args="); err == nil {
		if command := strings.TrimSpace(output); command != "" {
			return command
		}
	}
	return ""
}

func windowsProcessCommand(pid int) string {
	output, err := commandOutput("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "CommandLine", "/value")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CommandLine=") {
			return strings.TrimPrefix(line, "CommandLine=")
		}
	}
	return ""
}

func terminateProcess(pid int, port int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if _, err := commandOutput("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F"); err != nil {
			return err
		}
		return nil
	}
	_ = process.Signal(os.Interrupt)
	if waitPortFree("127.0.0.1", port, 1500*time.Millisecond) {
		return nil
	}
	if err := process.Kill(); err != nil {
		return err
	}
	if !waitPortFree("127.0.0.1", port, 1500*time.Millisecond) {
		return fmt.Errorf("端口仍被占用")
	}
	return nil
}

func waitPortFree(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsPortOccupied(host, port) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !IsPortOccupied(host, port)
}

func shortCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	base := fields[0]
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	return base
}

func commandOutput(name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
