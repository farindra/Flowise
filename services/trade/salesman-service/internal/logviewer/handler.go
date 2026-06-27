package logviewer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Handler struct {
	salesmanlLogPath string // error.log file
	flowiseLogsDir   string // Flowise logs directory
}

func NewHandler(salesmanLogPath, flowiseLogsDir string) *Handler {
	return &Handler{
		salesmanlLogPath: salesmanLogPath,
		flowiseLogsDir:   flowiseLogsDir,
	}
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Code    string `json:"code,omitempty"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// HandleUI serves the search UI.
func (h *Handler) HandleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, logUI)
}

// HandleSearch returns matching log entries as JSON.
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("q")))
	date := r.URL.Query().Get("date") // YYYY-MM-DD, empty = today
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	if q == "" {
		json.NewEncoder(w).Encode(map[string]any{"error": "parameter q kosong"})
		return
	}

	var results []LogEntry

	// 1. Search salesman error.log
	results = append(results, h.searchFile(h.salesmanlLogPath, q, date, "salesman-service")...)

	// 2. Search Flowise server logs
	if h.flowiseLogsDir != "" {
		pattern := filepath.Join(h.flowiseLogsDir, "server.log."+date+"*")
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		for _, f := range matches {
			results = append(results, h.searchFile(f, q, date, "flowise-server")...)
		}

		// Also search audit logs (JSONL)
		auditPattern := filepath.Join(h.flowiseLogsDir, "audit-"+date+"*.log.jsonl")
		auditMatches, _ := filepath.Glob(auditPattern)
		sort.Strings(auditMatches)
		for _, f := range auditMatches {
			results = append(results, h.searchAuditFile(f, q)...)
		}
	}

	// Sort by time desc
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time > results[j].Time
	})

	json.NewEncoder(w).Encode(map[string]any{
		"query":   q,
		"date":    date,
		"total":   len(results),
		"results": results,
	})
}

func (h *Handler) searchFile(path, q, date, source string) []LogEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var results []LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(strings.ToUpper(line), q) {
			continue
		}
		entry := parseLine(line, source)
		results = append(results, entry)
	}
	return results
}

func (h *Handler) searchAuditFile(path, q string) []LogEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var results []LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(strings.ToUpper(line), q) {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		ts, _ := obj["timestamp"].(string)
		msg := fmt.Sprintf("%v", obj)
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
		results = append(results, LogEntry{
			Time:    ts,
			Level:   "INFO",
			Source:  "flowise-audit",
			Message: line,
		})
	}
	return results
}

func parseLine(line, source string) LogEntry {
	entry := LogEntry{Source: source, Message: line, Level: "INFO"}

	// Extract error code [XXXXX]
	if i := strings.Index(line, " [ERROR] ["); i >= 0 {
		entry.Level = "ERROR"
		rest := line[i+10:]
		if j := strings.Index(rest, "]"); j >= 0 {
			entry.Code = rest[:j]
		}
	} else if strings.Contains(strings.ToUpper(line), "[ERROR]") || strings.Contains(strings.ToUpper(line), "ERROR:") {
		entry.Level = "ERROR"
	} else if strings.Contains(strings.ToUpper(line), "[WARN]") {
		entry.Level = "WARN"
	}

	// Extract timestamp (first word if it looks like ISO time)
	parts := strings.SplitN(line, " ", 3)
	if len(parts) >= 1 && (strings.Contains(parts[0], "T") || strings.Contains(parts[0], "-")) {
		entry.Time = parts[0]
		if len(parts) >= 3 {
			entry.Message = strings.Join(parts[1:], " ")
		}
	}

	return entry
}

// logUI is the embedded single-page HTML app.
const logUI = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Log Viewer — Ocean Bearings</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f1117;color:#e2e8f0;min-height:100vh}
.header{background:#1a1d2e;border-bottom:1px solid #2d3748;padding:16px 24px;display:flex;align-items:center;gap:12px}
.header h1{font-size:18px;font-weight:600;color:#fff}
.header .badge{background:#3182ce;color:#fff;font-size:11px;padding:2px 8px;border-radius:10px}
.search-box{padding:24px;background:#141720;border-bottom:1px solid #2d3748}
.search-row{display:flex;gap:10px;flex-wrap:wrap}
.search-row input{background:#1e2130;border:1px solid #2d3748;color:#e2e8f0;padding:10px 14px;border-radius:8px;font-size:14px;outline:none;transition:border .2s}
.search-row input:focus{border-color:#3182ce}
#q{flex:1;min-width:200px;font-family:monospace;font-size:15px;letter-spacing:.05em;text-transform:uppercase}
#date{width:160px}
.btn{background:#3182ce;color:#fff;border:none;padding:10px 20px;border-radius:8px;cursor:pointer;font-size:14px;font-weight:500;transition:background .2s}
.btn:hover{background:#2b6cb0}
.btn:disabled{background:#4a5568;cursor:not-allowed}
.hint{margin-top:10px;font-size:12px;color:#718096}
.results{padding:16px 24px}
.meta{color:#718096;font-size:13px;margin-bottom:14px}
.entry{background:#1a1d2e;border:1px solid #2d3748;border-radius:8px;margin-bottom:10px;overflow:hidden}
.entry-header{display:flex;align-items:center;gap:10px;padding:10px 14px;cursor:pointer;user-select:none}
.entry-header:hover{background:#1e2536}
.badge-level{font-size:11px;font-weight:700;padding:2px 8px;border-radius:4px;white-space:nowrap}
.badge-level.ERROR{background:#742a2a;color:#fc8181}
.badge-level.WARN{background:#744210;color:#f6ad55}
.badge-level.INFO{background:#1a365d;color:#63b3ed}
.badge-source{font-size:11px;color:#718096;background:#2d3748;padding:2px 8px;border-radius:4px;white-space:nowrap}
.badge-code{font-family:monospace;font-size:13px;font-weight:700;color:#f6e05e;background:#2d3200;padding:2px 8px;border-radius:4px;letter-spacing:.1em}
.entry-time{font-size:12px;color:#718096;margin-left:auto;white-space:nowrap}
.entry-msg{font-size:13px;color:#a0aec0;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.entry-body{display:none;padding:12px 14px;background:#111420;border-top:1px solid #2d3748;font-family:monospace;font-size:12px;color:#a0aec0;word-break:break-all;white-space:pre-wrap;max-height:300px;overflow-y:auto}
.entry.open .entry-body{display:block}
.entry.open .entry-header{background:#1e2536}
.empty{text-align:center;padding:48px;color:#4a5568}
.spinner{display:inline-block;width:16px;height:16px;border:2px solid #3182ce;border-top-color:transparent;border-radius:50%;animation:spin .7s linear infinite;vertical-align:middle;margin-right:8px}
@keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body>
<div class="header">
  <h1>🔍 Log Viewer</h1>
  <span class="badge">Ocean Bearings</span>
</div>
<div class="search-box">
  <div class="search-row">
    <input id="q" type="text" placeholder="Cari error code (misal: A3K9M) atau keyword..." autocomplete="off" autofocus>
    <input id="date" type="date" value="">
    <button class="btn" id="btn" onclick="search()">Cari</button>
  </div>
  <p class="hint">Cari by error code 5 huruf, pesan error, atau keyword lain. Tekan Enter untuk cari.</p>
</div>
<div class="results" id="results"></div>

<script>
document.getElementById('date').value = new Date().toISOString().slice(0,10);

document.getElementById('q').addEventListener('keydown', e => {
  if (e.key === 'Enter') search();
  // auto-uppercase
  setTimeout(() => { e.target.value = e.target.value.toUpperCase(); }, 0);
});

async function search() {
  const q = document.getElementById('q').value.trim();
  const date = document.getElementById('date').value;
  if (!q) { alert('Isi keyword atau error code dulu'); return; }

  const btn = document.getElementById('btn');
  const out = document.getElementById('results');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>Mencari...';
  out.innerHTML = '';

  try {
    const res = await fetch('/logs/search?q=' + encodeURIComponent(q) + '&date=' + date);
    const data = await res.json();
    if (data.error) { out.innerHTML = '<div class="empty">❌ ' + data.error + '</div>'; return; }
    if (!data.results || data.results.length === 0) {
      out.innerHTML = '<div class="empty">Tidak ada hasil untuk <b>' + q + '</b> pada ' + date + '</div>';
      return;
    }
    out.innerHTML = '<div class="meta">' + data.total + ' hasil untuk "' + data.query + '" — ' + data.date + '</div>' +
      data.results.map((r, i) => renderEntry(r, i)).join('');
  } catch(e) {
    out.innerHTML = '<div class="empty">❌ Gagal fetch: ' + e.message + '</div>';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Cari';
  }
}

function renderEntry(r, i) {
  const code = r.code ? '<span class="badge-code">' + r.code + '</span>' : '';
  const short = (r.message || '').replace(/</g,'&lt;').slice(0,120);
  const full  = (r.message || '').replace(/</g,'&lt;');
  return '<div class="entry" id="e'+i+'">'+
    '<div class="entry-header" onclick="toggle('+i+')">'+
      '<span class="badge-level '+(r.level||'INFO')+'">'+(r.level||'INFO')+'</span>'+
      '<span class="badge-source">'+(r.source||'')+'</span>'+
      code+
      '<span class="entry-msg">'+short+'</span>'+
      '<span class="entry-time">'+(r.time||'').replace('T',' ').replace('Z','')+'</span>'+
    '</div>'+
    '<div class="entry-body">'+full+'</div>'+
  '</div>';
}

function toggle(i) {
  const el = document.getElementById('e'+i);
  el.classList.toggle('open');
}
</script>
</body>
</html>`
