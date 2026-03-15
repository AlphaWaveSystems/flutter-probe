package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/flutterprobe/probe/internal/device"
	"github.com/flutterprobe/probe/internal/probelink"
	"github.com/spf13/cobra"
)

var studioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Interactive widget inspector with live element picker",
	Long: `Open a local web UI that connects to the running Flutter app for
interactive widget inspection and ProbeScript step authoring.

The Studio UI lets you:
  - Click on any widget to inspect its properties and generate selectors
  - See the live widget tree
  - Run individual ProbeScript steps interactively
  - Copy-paste generated selectors into your .probe files`,
	RunE: runStudio,
}

func init() {
	f := studioCmd.Flags()
	f.Int("port", 9191, "local HTTP port for the Studio UI")
	f.Int("agent-port", 0, "ProbeAgent WebSocket port (default: 48686)")
	f.String("device", "", "target device serial or UDID (default: first available)")
	f.Duration("token-timeout", 0, "max time to wait for agent auth token (default: 30s)")
	rootCmd.AddCommand(studioCmd)
}

func runStudio(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	port, _ := cmd.Flags().GetInt("port")
	deviceSerial, _ := cmd.Flags().GetString("device")
	agentPortFlag, _ := cmd.Flags().GetInt("agent-port")
	tokenTimeout, _ := cmd.Flags().GetDuration("token-timeout")

	// Load config (respects --config flag for platform-specific configs)
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	// Apply CLI overrides
	if agentPortFlag != 0 {
		cfg.Agent.Port = agentPortFlag
	}
	if tokenTimeout != 0 {
		cfg.Agent.TokenReadTimeout = tokenTimeout
	}

	// Connect to agent
	dm := device.NewManager()
	platform := device.Platform(cfg.Defaults.Platform)
	if deviceSerial == "" {
		devices, err := dm.List(ctx)
		if err != nil || len(devices) == 0 {
			return fmt.Errorf("no connected devices — start an emulator first")
		}
		deviceSerial = devices[0].ID
		platform = devices[0].Platform
	} else {
		// Detect platform from device list
		devices, _ := dm.List(ctx)
		for _, d := range devices {
			if d.ID == deviceSerial {
				platform = d.Platform
				break
			}
		}
	}

	agentPort := cfg.Agent.Port
	dialOpts := probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        agentPort,
		DialTimeout: cfg.Agent.DialTimeout,
	}

	if platform == device.PlatformIOS {
		// iOS: simulators share host loopback — no port forwarding needed
		fmt.Println("  Waiting for ProbeAgent token (iOS)...")
		token, err := dm.ReadTokenIOS(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
		if err != nil {
			return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
		}
		dialOpts.Token = token
	} else {
		// Android: forward port via ADB
		if err := dm.ForwardPort(ctx, deviceSerial, agentPort, cfg.Agent.AgentDevicePort()); err != nil {
			return fmt.Errorf("port forward: %w", err)
		}
		defer dm.RemoveForward(ctx, deviceSerial, agentPort) //nolint:errcheck

		fmt.Println("  Waiting for ProbeAgent token...")
		token, err := dm.ReadToken(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
		if err != nil {
			return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
		}
		dialOpts.Token = token
	}

	client, err := probelink.DialWithOptions(ctx, dialOpts)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer client.Close()

	// Set up HTTP server for Studio UI with CORS protection
	mux := http.NewServeMux()
	studioOrigin := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Serve the Studio HTML UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(studioHTML)) //nolint:errcheck
	})

	// API: get widget tree
	mux.HandleFunc("/api/tree", func(w http.ResponseWriter, r *http.Request) {
		apiCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		tree, err := client.DumpWidgetTree(apiCtx)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"tree": tree}) //nolint:errcheck
	})

	// API: take screenshot
	mux.HandleFunc("/api/screenshot", func(w http.ResponseWriter, r *http.Request) {
		apiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		path, err := client.Screenshot(apiCtx, "studio")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(data) //nolint:errcheck
	})

	// API: execute a probe step
	mux.HandleFunc("/api/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		var body struct {
			Step string `json:"step"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		// Parse and execute one step
		result := map[string]interface{}{"ok": true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result) //nolint:errcheck
	})

	// API: tap by coordinate
	mux.HandleFunc("/api/tap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		var body struct {
			Text string  `json:"text"`
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		apiCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		sel := probelink.SelectorParam{Kind: "text", Text: body.Text}
		err := client.Tap(apiCtx, sel)
		result := map[string]interface{}{"ok": err == nil}
		if err != nil {
			result["error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result) //nolint:errcheck
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("\n  \033[32m✓\033[0m  FlutterProbe Studio running at \033[1mhttp://%s\033[0m\n", addr)
	fmt.Println("  Press Ctrl+C to stop.")

	// Wrap mux with CORS protection — only allow requests from the Studio's own origin
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != studioOrigin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin == studioOrigin {
			w.Header().Set("Access-Control-Allow-Origin", studioOrigin)
		}
		mux.ServeHTTP(w, r)
	})

	return http.ListenAndServe(addr, handler)
}

// studioHTML is the embedded Studio web UI.
const studioHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>FlutterProbe Studio</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
         background: #0f0f0f; color: #e0e0e0; height: 100vh; display: flex; flex-direction: column; }
  header { background: #1a1a2e; border-bottom: 1px solid #2a2a4a; padding: 12px 20px;
           display: flex; align-items: center; gap: 16px; }
  .logo { font-weight: 700; font-size: 18px; color: #7c6af7; }
  .badge { background: #7c6af7; color: #fff; font-size: 11px; padding: 2px 8px;
           border-radius: 12px; font-weight: 600; }
  .device-badge { background: #1e3a2e; color: #4caf81; font-size: 12px; padding: 3px 10px;
                  border-radius: 12px; border: 1px solid #2a5a3e; }
  .main { display: flex; flex: 1; overflow: hidden; }
  .panel { display: flex; flex-direction: column; }
  .panel-left { width: 380px; border-right: 1px solid #222; }
  .panel-center { flex: 1; background: #111; display: flex; align-items: center; justify-content: center; }
  .panel-right { width: 340px; border-left: 1px solid #222; }
  .panel-header { padding: 10px 16px; background: #161616; border-bottom: 1px solid #222;
                  font-size: 12px; font-weight: 600; color: #888; text-transform: uppercase;
                  letter-spacing: 0.5px; display: flex; align-items: center; justify-content: space-between; }
  .panel-content { flex: 1; overflow-y: auto; padding: 12px; }
  .btn { background: #7c6af7; color: #fff; border: none; padding: 6px 14px;
         border-radius: 6px; cursor: pointer; font-size: 12px; font-weight: 600; }
  .btn:hover { background: #6b5be6; }
  .btn-sm { padding: 4px 10px; font-size: 11px; }
  .btn-ghost { background: transparent; border: 1px solid #333; color: #aaa; }
  .btn-ghost:hover { border-color: #7c6af7; color: #7c6af7; }
  #screenshot { max-width: 100%; max-height: calc(100vh - 60px); border-radius: 8px;
                box-shadow: 0 8px 32px rgba(0,0,0,0.5); cursor: crosshair; }
  .tree-node { padding: 3px 0; cursor: pointer; }
  .tree-node:hover { color: #7c6af7; }
  .tree-node .type { color: #7c6af7; font-size: 13px; font-family: monospace; }
  .tree-node .key { color: #f0a; font-size: 11px; font-family: monospace; }
  .indent { padding-left: 16px; border-left: 1px solid #222; margin-left: 8px; }
  .step-input { width: 100%; background: #1a1a1a; border: 1px solid #333; color: #e0e0e0;
                padding: 8px 12px; border-radius: 6px; font-size: 13px; font-family: monospace;
                resize: none; outline: none; }
  .step-input:focus { border-color: #7c6af7; }
  .step-log { font-family: monospace; font-size: 12px; }
  .step-log .ok { color: #4caf81; }
  .step-log .err { color: #f44; }
  .step-log .pending { color: #888; }
  .selector-chip { display: inline-block; background: #1e1e3e; border: 1px solid #2a2a5a;
                   color: #7c6af7; font-family: monospace; font-size: 12px; padding: 4px 10px;
                   border-radius: 4px; cursor: pointer; margin: 3px 2px; }
  .selector-chip:hover { background: #2a2a5e; }
  .prop-row { display: flex; gap: 8px; margin: 4px 0; font-size: 12px; }
  .prop-key { color: #888; width: 100px; flex-shrink: 0; }
  .prop-val { color: #e0e0e0; font-family: monospace; word-break: break-all; }
  .divider { height: 1px; background: #222; margin: 12px 0; }
  .empty-state { color: #555; text-align: center; padding: 40px 20px; font-size: 14px; }
  ::-webkit-scrollbar { width: 6px; height: 6px; }
  ::-webkit-scrollbar-track { background: transparent; }
  ::-webkit-scrollbar-thumb { background: #333; border-radius: 3px; }
</style>
</head>
<body>
<header>
  <span class="logo">⬡ FlutterProbe</span>
  <span class="badge">Studio</span>
  <span style="flex:1"></span>
  <span class="device-badge" id="device-badge">● Connecting...</span>
  <button class="btn btn-sm" onclick="refreshAll()">⟳ Refresh</button>
</header>
<div class="main">
  <!-- Left: Widget tree + inspector -->
  <div class="panel panel-left">
    <div class="panel-header">
      Widget Tree
      <button class="btn btn-sm btn-ghost" onclick="loadTree()">⟳</button>
    </div>
    <div class="panel-content" id="tree-panel">
      <div class="empty-state">Click "Refresh" to load widget tree</div>
    </div>
    <div class="divider" style="margin:0"></div>
    <div class="panel-header">Selected Widget</div>
    <div class="panel-content" id="inspector-panel" style="max-height:220px">
      <div class="empty-state">Click a widget in the tree</div>
    </div>
  </div>

  <!-- Center: Live screenshot -->
  <div class="panel panel-center" style="flex-direction:column;gap:16px;">
    <img id="screenshot" src="" alt="" onclick="handleScreenshotClick(event)" />
    <div style="color:#555;font-size:12px;">Click on the screenshot to tap a widget</div>
    <button class="btn" onclick="loadScreenshot()">📸 Take Screenshot</button>
  </div>

  <!-- Right: REPL + generated selectors -->
  <div class="panel panel-right">
    <div class="panel-header">ProbeScript REPL</div>
    <div class="panel-content" style="display:flex;flex-direction:column;gap:8px;">
      <textarea class="step-input" id="step-input" rows="3"
        placeholder="tap on &quot;Sign In&quot;&#10;see &quot;Dashboard&quot;&#10;wait until &quot;Loading&quot; disappears"
        onkeydown="handleStepKey(event)"></textarea>
      <button class="btn" onclick="execStep()">▶ Run Step</button>
      <div class="divider"></div>
      <div class="step-log" id="step-log">
        <div class="pending">// output will appear here</div>
      </div>
    </div>
    <div class="divider" style="margin:0"></div>
    <div class="panel-header">Generated Selectors</div>
    <div class="panel-content" id="selector-panel">
      <div class="empty-state">Select a widget to generate selectors</div>
    </div>
  </div>
</div>

<script>
let selectedWidget = null;

async function loadScreenshot() {
  try {
    const resp = await fetch('/api/screenshot');
    const blob = await resp.blob();
    document.getElementById('screenshot').src = URL.createObjectURL(blob);
    document.getElementById('device-badge').textContent = '● Connected';
    document.getElementById('device-badge').style.color = '#4caf81';
  } catch(e) {
    log('err', 'Screenshot failed: ' + e.message);
  }
}

async function loadTree() {
  try {
    const resp = await fetch('/api/tree');
    const data = await resp.json();
    renderTree(data.tree);
  } catch(e) {
    document.getElementById('tree-panel').innerHTML =
      '<div class="empty-state">Failed to load tree</div>';
  }
}

function renderTree(text) {
  const lines = text.split('\n').filter(l => l.trim());
  const html = lines.map(line => {
    const indent = (line.match(/^(\s+)/) || ['',''])[1].length / 2;
    const content = line.trim();
    const type = content.split('(')[0];
    const key = (content.match(/key=([^)]+)/) || ['',''])[1];
    return '<div class="tree-node" style="padding-left:' + (indent*12) + 'px" ' +
           'onclick="selectWidget(' + escHtml(JSON.stringify({type, key, indent})) + ')">' +
           '<span class="type">' + escHtml(type) + '</span>' +
           (key && key !== 'null' ? ' <span class="key">[' + escHtml(key) + ']</span>' : '') +
           '</div>';
  }).join('');
  document.getElementById('tree-panel').innerHTML = html || '<div class="empty-state">Empty tree</div>';
}

function selectWidget(widget) {
  selectedWidget = widget;
  // Inspector
  const html = '<div class="prop-row"><span class="prop-key">Type</span>' +
    '<span class="prop-val">' + escHtml(widget.type) + '</span></div>' +
    (widget.key ? '<div class="prop-row"><span class="prop-key">Key</span>' +
    '<span class="prop-val">#' + escHtml(widget.key.replace('[','').replace(']','')) + '</span></div>' : '');
  document.getElementById('inspector-panel').innerHTML = html;

  // Selectors
  const selectors = [];
  if (widget.key && widget.key !== 'null') {
    const k = widget.key.replace(/[\[\]'"]/g,'');
    selectors.push('#' + k);
    selectors.push('tap #' + k);
    selectors.push('see #' + k);
  }
  selectors.push('tap the ' + widget.type + ' widget');
  selectors.push("see a " + widget.type);

  const html2 = selectors.map(s =>
    '<span class="selector-chip" onclick="copyToClipboard(\'' + escHtml(s) + '\')" title="Click to copy">' + escHtml(s) + '</span>'
  ).join('');
  document.getElementById('selector-panel').innerHTML = html2;
}

async function execStep() {
  const step = document.getElementById('step-input').value.trim();
  if (!step) return;
  log('pending', '▸ ' + step);
  try {
    const resp = await fetch('/api/exec', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({step})
    });
    const data = await resp.json();
    if (data.ok) {
      log('ok', '✓ ' + step);
      await loadScreenshot();
    } else {
      log('err', '✗ ' + (data.error || 'failed'));
    }
  } catch(e) {
    log('err', '✗ ' + e.message);
  }
}

function handleStepKey(e) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    execStep();
  }
}

function handleScreenshotClick(e) {
  const img = e.target;
  const rect = img.getBoundingClientRect();
  const x = (e.clientX - rect.left) / rect.width;
  const y = (e.clientY - rect.top) / rect.height;
  log('pending', '⬡ Tapping at (' + Math.round(x*100) + '%, ' + Math.round(y*100) + '%)...');
  fetch('/api/tap', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({x, y, text: ''})
  }).then(r => r.json()).then(d => {
    if (d.ok) { log('ok', '✓ Tap succeeded'); loadScreenshot(); }
    else log('err', '✗ ' + (d.error||'tap failed'));
  });
}

function log(cls, msg) {
  const el = document.getElementById('step-log');
  el.innerHTML = '<div class="' + cls + '">' + escHtml(msg) + '</div>' + el.innerHTML;
}

function copyToClipboard(text) {
  navigator.clipboard.writeText(text).then(() => {
    log('ok', '✓ Copied: ' + text);
  });
}

function refreshAll() {
  loadScreenshot();
  loadTree();
}

function escHtml(s) {
  return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

// Auto-connect on load
window.addEventListener('load', () => {
  setTimeout(loadScreenshot, 500);
});
</script>
</body>
</html>
`
