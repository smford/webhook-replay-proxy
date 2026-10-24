package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) registerUIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/_dashboard", s.handleDashboard)
	mux.HandleFunc("/api/events", s.handleAPIEvents)
	mux.HandleFunc("/api/replay", s.handleAPIReplay)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	events, err := s.cfg.Store.ListEvents(50)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	type eventSummary struct {
		ID            string              `json:"id"`
		Timestamp     time.Time           `json:"timestamp"`
		Provider      string              `json:"provider"`
		Method        string              `json:"method"`
		Path          string              `json:"path"`
		Query         string              `json:"query"`
		TargetURL     string              `json:"target_url"`
		ForwardStatus int                 `json:"forward_status"`
		LatencyMs     int64               `json:"latency_ms"`
		Headers       map[string][]string `json:"headers"`
		Body          string              `json:"body"`
	}

	summaries := make([]eventSummary, len(events))
	for i, e := range events {
		summaries[i] = eventSummary{
			ID:            e.ID,
			Timestamp:     e.Timestamp,
			Provider:      e.Provider,
			Method:        e.Method,
			Path:          e.Path,
			Query:         e.Query,
			TargetURL:     e.TargetURL,
			ForwardStatus: e.ForwardStatus,
			LatencyMs:     e.LatencyMs,
			Headers:       e.Headers,
			Body:          string(e.Body),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summaries)
}

type replayAPIRequest struct {
	EventID     string   `json:"event_id"`
	TargetURL   string   `json:"target_url"`
	Provider    string   `json:"provider"`
	Secret      string   `json:"secret"`
	Mutations   []string `json:"mutations"`
	Concurrency int      `json:"concurrency"`
	CorruptSig  bool     `json:"corrupt_sig"`
}

type replayAPIResponse struct {
	Success bool           `json:"success"`
	Results []ReplayResult `json:"results"`
	CurlCmd string         `json:"curl_cmd"`
	Error   string         `json:"error,omitempty"`
}

func (s *Server) handleAPIReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.Store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	var req replayAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid json: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	opts := ReplayOptions{
		EventID:     req.EventID,
		TargetURL:   req.TargetURL,
		Provider:    req.Provider,
		Secret:      req.Secret,
		Mutations:   req.Mutations,
		Concurrency: req.Concurrency,
		CorruptSig:  req.CorruptSig,
	}

	re := NewReplayEngine(s.cfg.Store)
	curlCmd, _ := re.GenerateCurl(opts)
	results, err := re.Execute(r.Context(), opts)

	resp := replayAPIResponse{
		Success: err == nil,
		Results: results,
		CurlCmd: curlCmd,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Webhook Replay Proxy | Dashboard</title>
  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --accent: #58a6ff;
      --green: #3fb950;
      --red: #f85149;
      --code-bg: #090d13;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: var(--bg);
      color: var(--text);
      display: flex;
      flex-direction: column;
      height: 100vh;
      overflow: hidden;
    }
    header {
      background: var(--card-bg);
      border-bottom: 1px solid var(--border);
      padding: 12px 24px;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .brand { font-size: 1.15rem; font-weight: 600; color: #fff; display: flex; align-items: center; gap: 8px; }
    .badge {
      background: rgba(88, 166, 255, 0.15);
      color: var(--accent);
      padding: 2px 8px;
      border-radius: 12px;
      font-size: 0.75rem;
      border: 1px solid rgba(88, 166, 255, 0.3);
    }
    .main { display: flex; flex: 1; overflow: hidden; }
    .sidebar {
      width: 360px;
      background: var(--card-bg);
      border-right: 1px solid var(--border);
      overflow-y: auto;
      display: flex;
      flex-direction: column;
    }
    .sidebar-header {
      padding: 12px 16px;
      font-size: 0.85rem;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--text-muted);
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .refresh-btn {
      background: transparent;
      border: 1px solid var(--border);
      color: var(--text);
      padding: 4px 8px;
      border-radius: 6px;
      cursor: pointer;
      font-size: 0.75rem;
    }
    .refresh-btn:hover { background: var(--border); }
    .event-item {
      padding: 12px 16px;
      border-bottom: 1px solid var(--border);
      cursor: pointer;
      transition: background 0.15s ease;
    }
    .event-item:hover, .event-item.active { background: rgba(88, 166, 255, 0.08); }
    .event-row { display: flex; justify-content: space-between; margin-bottom: 4px; font-size: 0.85rem; }
    .event-method { font-weight: bold; color: var(--accent); }
    .event-status { font-weight: 600; }
    .status-2xx { color: var(--green); }
    .status-err { color: var(--red); }
    .event-path { font-family: monospace; font-size: 0.8rem; color: var(--text); }
    .event-time { font-size: 0.75rem; color: var(--text-muted); }
    .content {
      flex: 1;
      overflow-y: auto;
      padding: 24px;
      display: flex;
      flex-direction: column;
      gap: 20px;
    }
    .card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 16px;
    }
    .card-title {
      font-size: 0.95rem;
      font-weight: 600;
      margin-bottom: 12px;
      color: #fff;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    pre {
      background: var(--code-bg);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 12px;
      font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
      font-size: 0.82rem;
      overflow-x: auto;
      white-space: pre-wrap;
      word-break: break-all;
    }
    .action-btn {
      background: var(--accent);
      color: #fff;
      border: none;
      padding: 6px 12px;
      border-radius: 6px;
      font-size: 0.8rem;
      font-weight: 500;
      cursor: pointer;
    }
    .action-btn:hover { opacity: 0.9; }
    .form-group { margin-bottom: 12px; }
    .form-label { display: block; font-size: 0.8rem; color: var(--text-muted); margin-bottom: 4px; }
    .form-input {
      width: 100%;
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 8px 12px;
      color: var(--text);
      font-size: 0.85rem;
    }
    .form-row { display: flex; gap: 12px; }
    .empty-state { text-align: center; color: var(--text-muted); margin: auto; }
  </style>
</head>
<body>
  <header>
    <div class="brand">
      <span>⚡ Webhook Replay Proxy</span>
      <span class="badge">Live Dashboard</span>
    </div>
    <div style="font-size: 0.85rem; color: var(--text-muted);">
      Pure-Go SQLite Flight Recorder
    </div>
  </header>
  <div class="main">
    <div class="sidebar">
      <div class="sidebar-header">
        <span>Captured Webhooks</span>
        <button class="refresh-btn" onclick="fetchEvents()">Refresh</button>
      </div>
      <div id="event-list"></div>
    </div>
    <div class="content" id="detail-panel">
      <div class="empty-state">
        <h3>Select a captured webhook from the left sidebar</h3>
        <p style="margin-top: 8px; font-size: 0.85rem;">Inspect headers, view decoded payloads, mutate JSON, or replay with fresh HMAC signatures.</p>
      </div>
    </div>
  </div>

  <script>
    let events = [];
    let selectedEvent = null;

    async function fetchEvents() {
      try {
        const res = await fetch('/api/events');
        events = await res.json();
        renderEventList();
      } catch (err) {
        console.error('Failed to load events', err);
      }
    }

    function renderEventList() {
      const container = document.getElementById('event-list');
      if (!events || events.length === 0) {
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-muted); font-size: 0.85rem;">No webhooks captured yet. Send requests to the proxy listener!</div>';
        return;
      }
      container.innerHTML = events.map(e => {
        const is2xx = e.forward_status >= 200 && e.forward_status < 300;
        const statusClass = e.forward_status === 0 ? '' : (is2xx ? 'status-2xx' : 'status-err');
        const statusText = e.forward_status === 0 ? 'Recorded' : e.forward_status;
        const active = selectedEvent && selectedEvent.id === e.id ? 'active' : '';
        const d = new Date(e.timestamp).toLocaleTimeString();
        return ` + "`" + `
          <div class="event-item ${active}" onclick="selectEvent('${e.id}')">
            <div class="event-row">
              <span class="event-method">${e.method}</span>
              <span class="event-status ${statusClass}">${statusText}</span>
            </div>
            <div class="event-path">${e.path}</div>
            <div class="event-row" style="margin-top: 6px;">
              <span style="font-size: 0.75rem; text-transform: uppercase; color: var(--accent);">${e.provider || 'auto'}</span>
              <span class="event-time">${d}</span>
            </div>
          </div>
        ` + "`" + `;
      }).join('');
    }

    function selectEvent(id) {
      selectedEvent = events.find(e => e.id === id);
      renderEventList();
      renderDetail();
    }

    function renderDetail() {
      const panel = document.getElementById('detail-panel');
      if (!selectedEvent) return;

      let formattedBody = selectedEvent.body;
      try {
        formattedBody = JSON.stringify(JSON.parse(selectedEvent.body), null, 2);
      } catch (_) {}

      panel.innerHTML = ` + "`" + `
        <div class="card">
          <div class="card-title">
            <span>Event: <code>${selectedEvent.id}</code></span>
            <span class="badge">${selectedEvent.provider || 'auto'}</span>
          </div>
          <div style="font-size: 0.85rem; display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 12px;">
            <div><strong>Timestamp:</strong> ${new Date(selectedEvent.timestamp).toISOString()}</div>
            <div><strong>Forward Status:</strong> ${selectedEvent.forward_status}</div>
            <div><strong>Latency:</strong> ${selectedEvent.latency_ms}ms</div>
          </div>
        </div>

        <div class="card">
          <div class="card-title">Interactive Replay & Mutation</div>
          <div class="form-row">
            <div class="form-group" style="flex: 2;">
              <label class="form-label">Destination Target URL</label>
              <input type="text" id="replay-target" class="form-input" value="${selectedEvent.target_url || 'http://localhost:3000/webhook'}">
            </div>
            <div class="form-group" style="flex: 1;">
              <label class="form-label">HMAC Secret (optional)</label>
              <input type="text" id="replay-secret" class="form-input" placeholder="whsec_...">
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">Mutate JSON (comma-separated, e.g. data.object.status=failed)</label>
            <input type="text" id="replay-mutations" class="form-input" placeholder="data.object.status=failed">
          </div>
          <div style="display: flex; gap: 12px; align-items: center;">
            <button class="action-btn" onclick="executeReplay()">🚀 Replay Event</button>
            <button class="refresh-btn" onclick="copyCurl()">📋 Copy curl</button>
            <span id="replay-status" style="font-size: 0.85rem;"></span>
          </div>
        </div>

        <div class="card">
          <div class="card-title">Payload Body</div>
          <pre><code>${escapeHtml(formattedBody)}</code></pre>
        </div>

        <div class="card">
          <div class="card-title">Headers</div>
          <pre><code>${escapeHtml(JSON.stringify(selectedEvent.headers, null, 2))}</code></pre>
        </div>
      ` + "`" + `;
    }

    async function executeReplay() {
      const statusEl = document.getElementById('replay-status');
      statusEl.textContent = 'Replaying...';
      const target = document.getElementById('replay-target').value;
      const secret = document.getElementById('replay-secret').value;
      const mutStr = document.getElementById('replay-mutations').value;
      const mutations = mutStr ? mutStr.split(',').map(s => s.trim()).filter(Boolean) : [];

      try {
        const res = await fetch('/api/replay', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            event_id: selectedEvent.id,
            target_url: target,
            provider: selectedEvent.provider,
            secret: secret,
            mutations: mutations,
            concurrency: 1
          })
        });
        const data = await res.json();
        if (data.success && data.results.length > 0) {
          const r = data.results[0];
          statusEl.innerHTML = '<span style="color: var(--green)">✓ Dispatched! Status: ' + r.StatusCode + ' (' + r.LatencyMs + 'ms)</span>';
        } else {
          statusEl.innerHTML = '<span style="color: var(--red)">✗ ' + (data.error || 'Failed') + '</span>';
        }
      } catch (err) {
        statusEl.innerHTML = '<span style="color: var(--red)">✗ ' + err.message + '</span>';
      }
    }

    function copyCurl() {
      const target = document.getElementById('replay-target').value;
      const cmd = ` + "`" + `curl -X POST "${target}" -H "Content-Type: application/json" -d '${selectedEvent.body}'` + "`" + `;
      navigator.clipboard.writeText(cmd);
      const statusEl = document.getElementById('replay-status');
      statusEl.innerHTML = '<span style="color: var(--green)">✓ Copied curl to clipboard!</span>';
    }

    function escapeHtml(str) {
      return (str || '').replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }

    fetchEvents();
    setInterval(fetchEvents, 5000);
  </script>
</body>
</html>
`
