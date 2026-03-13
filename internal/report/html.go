// Package report generates interactive HTML test reports.
package report

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/flutterprobe/probe/internal/runner"
)

// HTMLReport generates an interactive HTML dashboard from test results.
type HTMLReport struct {
	OutputPath  string
	ProjectName string
}

// NewHTMLReport creates an HTMLReport.
func NewHTMLReport(outputPath, projectName string) *HTMLReport {
	return &HTMLReport{OutputPath: outputPath, ProjectName: projectName}
}

// Write renders the report to the output path.
func (h *HTMLReport) Write(results []runner.TestResult, artifacts map[string][]string) error {
	if err := os.MkdirAll(filepath.Dir(h.OutputPath), 0755); err != nil {
		return err
	}

	// Build JSON data for charts
	passed, failed, skipped := 0, 0, 0
	var totalDur time.Duration
	type resultJSON struct {
		Name     string   `json:"name"`
		File     string   `json:"file"`
		Passed   bool     `json:"passed"`
		Skipped  bool     `json:"skipped"`
		DurMs    int64    `json:"dur_ms"`
		Error    string   `json:"error,omitempty"`
		Shots    []string `json:"shots,omitempty"`
	}
	var rows []resultJSON
	for _, r := range results {
		totalDur += r.Duration
		switch {
		case r.Skipped:
			skipped++
		case r.Passed:
			passed++
		default:
			failed++
		}
		rj := resultJSON{
			Name:    r.TestName,
			File:    r.File,
			Passed:  r.Passed,
			Skipped: r.Skipped,
			DurMs:   r.Duration.Milliseconds(),
		}
		if r.Error != nil {
			rj.Error = r.Error.Error()
		}
		rj.Shots = artifacts[r.TestName]
		rows = append(rows, rj)
	}

	resultsJSON, err := json.Marshal(rows)
	if err != nil {
		return err
	}

	data := struct {
		ProjectName string
		GeneratedAt string
		TotalDur    string
		Passed      int
		Failed      int
		Skipped     int
		Total       int
		ResultsJSON string
	}{
		ProjectName: h.ProjectName,
		GeneratedAt: time.Now().Format("Mon Jan 02 2006 15:04:05 MST"),
		TotalDur:    totalDur.Round(time.Millisecond).String(),
		Passed:      passed,
		Failed:      failed,
		Skipped:     skipped,
		Total:       len(results),
		ResultsJSON: string(resultsJSON),
	}

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"escape": html.EscapeString,
		"join":   strings.Join,
		"sprintf": fmt.Sprintf,
	}).Parse(reportTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(h.OutputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// Open opens the HTML report in the default browser.
func (h *HTMLReport) Open() error {
	var cmd *exec.Cmd
	switch {
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", h.OutputPath)
	case commandExists("open"):
		cmd = exec.Command("open", h.OutputPath)
	default:
		fmt.Printf("Open %s in your browser\n", h.OutputPath)
		return nil
	}
	return cmd.Start()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.ProjectName}} — FlutterProbe Report</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f0f0f;color:#e0e0e0;min-height:100vh}
header{background:linear-gradient(135deg,#1a1a2e,#0f0f1e);padding:32px 40px;border-bottom:1px solid #1e1e3e}
.header-top{display:flex;align-items:center;gap:12px;margin-bottom:8px}
.logo{font-size:22px;font-weight:800;color:#7c6af7}
.project{font-size:16px;color:#aaa}
.meta{font-size:13px;color:#555;margin-top:4px}
.summary{display:grid;grid-template-columns:repeat(4,1fr);gap:16px;padding:24px 40px;background:#111;border-bottom:1px solid #1a1a1a}
.stat{background:#181828;border:1px solid #222;border-radius:10px;padding:16px 20px;text-align:center}
.stat .num{font-size:36px;font-weight:800;line-height:1}
.stat .lbl{font-size:12px;color:#666;margin-top:4px;text-transform:uppercase;letter-spacing:.5px}
.pass .num{color:#4caf81}.fail .num{color:#f44}.skip .num{color:#888}.dur .num{color:#7c6af7;font-size:28px}
.container{max-width:1200px;margin:0 auto;padding:32px 40px}
.filter-bar{display:flex;gap:8px;margin-bottom:24px;align-items:center}
.filter-btn{background:#1a1a1a;border:1px solid #333;color:#aaa;padding:6px 16px;border-radius:20px;cursor:pointer;font-size:13px;transition:.15s}
.filter-btn:hover,.filter-btn.active{background:#2a1a4e;border-color:#7c6af7;color:#7c6af7}
.search{flex:1;background:#1a1a1a;border:1px solid #333;color:#e0e0e0;padding:7px 14px;border-radius:20px;font-size:13px;outline:none}
.search:focus{border-color:#7c6af7}
.test-card{background:#161616;border:1px solid #222;border-radius:8px;margin-bottom:8px;overflow:hidden;transition:.15s}
.test-card:hover{border-color:#333}
.test-header{display:flex;align-items:center;padding:12px 16px;cursor:pointer;gap:12px}
.status-dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.pass-dot{background:#4caf81}.fail-dot{background:#f44}.skip-dot{background:#888}
.test-name{flex:1;font-size:14px;font-weight:500}
.test-file{font-size:12px;color:#555;font-family:monospace}
.test-dur{font-size:12px;color:#555;font-family:monospace;min-width:60px;text-align:right}
.test-detail{padding:0 16px 14px;display:none;border-top:1px solid #1a1a1a;margin-top:0}
.test-detail.open{display:block}
.error-msg{background:#1a0808;border:1px solid #3a1818;border-radius:6px;padding:12px;font-family:monospace;font-size:13px;color:#f88;margin-top:10px;white-space:pre-wrap}
.shots{display:flex;gap:8px;margin-top:10px;flex-wrap:wrap}
.shots img{max-height:160px;border-radius:6px;border:1px solid #333}
.bar{height:4px;background:#1a1a1a;border-radius:2px;margin-bottom:32px;overflow:hidden}
.bar-pass{height:100%;background:#4caf81;transition:width .5s}
.empty{text-align:center;color:#444;padding:60px;font-size:16px}
</style>
</head>
<body>
<header>
  <div class="header-top">
    <span class="logo">⬡ FlutterProbe</span>
    <span class="project">/ {{.ProjectName}}</span>
  </div>
  <div class="meta">Generated {{.GeneratedAt}} · Total duration {{.TotalDur}}</div>
</header>

<div class="summary">
  <div class="stat pass"><div class="num">{{.Passed}}</div><div class="lbl">Passed</div></div>
  <div class="stat fail"><div class="num">{{.Failed}}</div><div class="lbl">Failed</div></div>
  <div class="stat skip"><div class="num">{{.Skipped}}</div><div class="lbl">Skipped</div></div>
  <div class="stat dur"><div class="num">{{.TotalDur}}</div><div class="lbl">Total Time</div></div>
</div>

<div class="container">
  <div class="bar">
    <div class="bar-pass" id="bar-pass"></div>
  </div>

  <div class="filter-bar">
    <button class="filter-btn active" onclick="filter('all',this)">All ({{.Total}})</button>
    <button class="filter-btn" onclick="filter('pass',this)">Passed ({{.Passed}})</button>
    <button class="filter-btn" onclick="filter('fail',this)">Failed ({{.Failed}})</button>
    <button class="filter-btn" onclick="filter('skip',this)">Skipped ({{.Skipped}})</button>
    <input class="search" id="search" placeholder="Search tests..." oninput="search(this.value)">
  </div>

  <div id="test-list"></div>
</div>

<script>
const RESULTS = {{.ResultsJSON}};
let currentFilter = 'all';

function render(results) {
  const list = document.getElementById('test-list');
  if (!results.length) {
    list.innerHTML = '<div class="empty">No tests match</div>';
    return;
  }
  list.innerHTML = results.map((r, i) => {
    const cls = r.skipped ? 'skip' : r.passed ? 'pass' : 'fail';
    const dot = cls + '-dot';
    const shots = (r.shots||[]).map(s => '<img src="file://'+s+'" loading="lazy">').join('');
    const err = r.error ? '<div class="error-msg">'+escHtml(r.error)+'</div>' : '';
    const shotsHtml = shots ? '<div class="shots">'+shots+'</div>' : '';
    return '<div class="test-card '+cls+'" data-name="'+escHtml(r.name)+'" data-status="'+cls+'">' +
      '<div class="test-header" onclick="toggle('+i+')">' +
        '<span class="status-dot '+dot+'"></span>' +
        '<span class="test-name">'+escHtml(r.name)+'</span>' +
        '<span class="test-file">'+escHtml(r.file)+'</span>' +
        '<span class="test-dur">'+r.dur_ms+'ms</span>' +
      '</div>' +
      '<div class="test-detail" id="detail-'+i+'">' +
        (err||shotsHtml ? err+shotsHtml : '<div style="color:#555;font-size:13px;padding-top:8px">No additional details</div>') +
      '</div>' +
    '</div>';
  }).join('');

  // Pass bar
  const pct = RESULTS.length ? (RESULTS.filter(r=>r.passed).length / RESULTS.length * 100) : 0;
  document.getElementById('bar-pass').style.width = pct + '%';
}

function toggle(i) {
  const el = document.getElementById('detail-'+i);
  el.classList.toggle('open');
}

function filter(type, btn) {
  currentFilter = type;
  document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  applyFilters();
}

function search(q) {
  applyFilters(q);
}

function applyFilters(q) {
  q = q || document.getElementById('search').value.toLowerCase();
  let filtered = RESULTS;
  if (currentFilter === 'pass') filtered = filtered.filter(r => r.passed && !r.skipped);
  if (currentFilter === 'fail') filtered = filtered.filter(r => !r.passed && !r.skipped);
  if (currentFilter === 'skip') filtered = filtered.filter(r => r.skipped);
  if (q) filtered = filtered.filter(r => r.name.toLowerCase().includes(q) || r.file.toLowerCase().includes(q));
  render(filtered);
}

function escHtml(s) {
  return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

render(RESULTS);
</script>
</body>
</html>
`
