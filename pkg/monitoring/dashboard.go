// Package monitoring provides metrics collection and health monitoring for the distributed storage system.
package monitoring

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Dashboard provides a web-based monitoring interface.
type Dashboard struct {
	monitor *Monitor
	logger  *logrus.Logger
}

// NewDashboard creates a new monitoring dashboard.
func NewDashboard(monitor *Monitor, logger *logrus.Logger) *Dashboard {
	return &Dashboard{
		monitor: monitor,
		logger:  logger,
	}
}

// StartDashboardServer starts the web dashboard server.
func (d *Dashboard) StartDashboardServer(port int) {
	mux := http.NewServeMux()

	// Dashboard home page
	mux.HandleFunc("/", d.handleDashboard)

	// API endpoints for AJAX updates
	mux.HandleFunc("/api/metrics", d.handleAPIMetrics)
	mux.HandleFunc("/api/health", d.handleAPIHealth)
	mux.HandleFunc("/api/cluster", d.handleAPICluster)

	// Static assets (CSS, JS)
	mux.HandleFunc("/static/", d.handleStatic)

	addr := fmt.Sprintf(":%d", port)
	d.logger.Infof("🖥️  Dashboard server starting on http://localhost%s", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			d.logger.WithError(err).Error("Dashboard server failed")
		}
	}()
}

// handleDashboard serves the main dashboard page.
func (d *Dashboard) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))

	data := struct {
		NodeID    string
		Timestamp string
	}{
		NodeID:    d.monitor.metrics.nodeID,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		d.logger.WithError(err).Error("Failed to render dashboard template")
	}
}

// handleAPIMetrics provides metrics data as JSON.
func (d *Dashboard) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	metrics := d.monitor.metrics.GetNodeMetrics()
	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAPIHealth provides health data as JSON.
func (d *Dashboard) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	health := d.monitor.GetClusterHealth()
	if err := json.NewEncoder(w).Encode(health); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAPICluster provides cluster overview data.
func (d *Dashboard) handleAPICluster(w http.ResponseWriter, r *http.Request) {
	health := d.monitor.GetClusterHealth()
	metrics := d.monitor.metrics.GetNodeMetrics()

	clusterInfo := map[string]interface{}{
		"health":  health,
		"metrics": metrics,
		"summary": map[string]interface{}{
			"total_nodes":   len(health.Peers) + 1, // +1 for current node
			"leader_node":   health.Leader,
			"current_state": health.State,
			"files_tracked": metrics.FilesReplicated,
			"total_bytes":   metrics.BytesReplicated,
			"error_rate":    float64(metrics.ReplicationErrors) / float64(metrics.FilesReplicated+1) * 100,
			"avg_repl_time": metrics.AvgReplicationTime,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(clusterInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleStatic serves static CSS/JS files.
func (d *Dashboard) handleStatic(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/static/dashboard.css":
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(dashboardCSS))
	case "/static/dashboard.js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(dashboardJS))
	default:
		http.NotFound(w, r)
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Pickbox Distributed Storage - Dashboard</title>
    <link rel="stylesheet" href="/static/dashboard.css">
    <script src="/static/dashboard.js"></script>
</head>
<body>
    <div class="container">
        <header>
            <h1>🗂️ Pickbox Distributed Storage</h1>
            <div class="header-info">
                <span class="node-id">Node: {{.NodeID}}</span>
                <span class="timestamp">{{.Timestamp}}</span>
            </div>
        </header>

        <div class="dashboard-grid">
            <!-- Cluster Health Card -->
            <div class="card">
                <h2>🏥 Cluster Health</h2>
                <div class="metrics-grid">
                    <div class="metric">
                        <span class="metric-label">State</span>
                        <span class="metric-value" id="node-state">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Leader</span>
                        <span class="metric-value" id="leader-node">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Peers</span>
                        <span class="metric-value" id="peer-count">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Log Index</span>
                        <span class="metric-value" id="log-index">-</span>
                    </div>
                </div>
            </div>

            <!-- Replication Metrics Card -->
            <div class="card">
                <h2>🔄 Replication Metrics</h2>
                <div class="metrics-grid">
                    <div class="metric">
                        <span class="metric-label">Files Replicated</span>
                        <span class="metric-value" id="files-replicated">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Bytes Transferred</span>
                        <span class="metric-value" id="bytes-replicated">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Avg Repl Time</span>
                        <span class="metric-value" id="avg-repl-time">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Error Rate</span>
                        <span class="metric-value" id="error-rate">-</span>
                    </div>
                </div>
            </div>

            <!-- System Resources Card -->
            <div class="card">
                <h2>💻 System Resources</h2>
                <div class="metrics-grid">
                    <div class="metric">
                        <span class="metric-label">Memory Usage</span>
                        <span class="metric-value" id="memory-usage">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Goroutines</span>
                        <span class="metric-value" id="goroutines">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Uptime</span>
                        <span class="metric-value" id="uptime">-</span>
                    </div>
                    <div class="metric">
                        <span class="metric-label">Last Replication</span>
                        <span class="metric-value" id="last-replication">-</span>
                    </div>
                </div>
            </div>

            <!-- Recent Activity Card -->
            <div class="card full-width">
                <h2>📋 Cluster Status</h2>
                <div class="status-indicator">
                    <div class="status-light" id="status-light"></div>
                    <span id="status-text">Checking...</span>
                </div>
                <div class="peer-list" id="peer-list">
                    <!-- Peers will be populated by JavaScript -->
                </div>
            </div>
        </div>

        <footer>
            <p>🚀 Pickbox Distributed Storage System | Auto-refresh every 5 seconds</p>
        </footer>
    </div>
</body>
</html>`

const dashboardCSS = `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    min-height: 100vh;
    padding: 20px;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
}

header {
    background: rgba(255, 255, 255, 0.95);
    padding: 20px;
    border-radius: 10px;
    margin-bottom: 30px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

header h1 {
    color: #333;
    font-size: 2em;
}

.header-info {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 5px;
}

.node-id, .timestamp {
    background: #4CAF50;
    color: white;
    padding: 5px 10px;
    border-radius: 15px;
    font-size: 0.9em;
    font-weight: bold;
}

.timestamp {
    background: #2196F3;
}

.dashboard-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 20px;
    margin-bottom: 30px;
}

.card {
    background: rgba(255, 255, 255, 0.95);
    padding: 25px;
    border-radius: 10px;
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
    transition: transform 0.2s ease;
}

.card:hover {
    transform: translateY(-2px);
}

.card.full-width {
    grid-column: 1 / -1;
}

.card h2 {
    color: #333;
    margin-bottom: 20px;
    font-size: 1.3em;
    border-bottom: 2px solid #eee;
    padding-bottom: 10px;
}

.metrics-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 15px;
}

.metric {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    padding: 15px;
    background: #f8f9fa;
    border-radius: 8px;
    border: 1px solid #e9ecef;
}

.metric-label {
    font-size: 0.9em;
    color: #666;
    margin-bottom: 5px;
    font-weight: 600;
}

.metric-value {
    font-size: 1.4em;
    font-weight: bold;
    color: #333;
}

.status-indicator {
    display: flex;
    align-items: center;
    gap: 15px;
    margin-bottom: 20px;
    padding: 15px;
    background: #f8f9fa;
    border-radius: 8px;
}

.status-light {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: #ffc107;
    animation: pulse 2s infinite;
}

.status-light.healthy {
    background: #28a745;
}

.status-light.unhealthy {
    background: #dc3545;
}

@keyframes pulse {
    0% { opacity: 1; }
    50% { opacity: 0.5; }
    100% { opacity: 1; }
}

.peer-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 10px;
}

.peer-item {
    padding: 10px;
    background: white;
    border-radius: 5px;
    border-left: 4px solid #007bff;
    font-family: monospace;
    font-size: 0.9em;
}

footer {
    text-align: center;
    color: rgba(255, 255, 255, 0.8);
    margin-top: 30px;
    font-size: 0.9em;
}

/* Responsive adjustments */
@media (max-width: 768px) {
    .dashboard-grid {
        grid-template-columns: 1fr;
    }
    
    .metrics-grid {
        grid-template-columns: 1fr;
    }
    
    header {
        flex-direction: column;
        gap: 15px;
        text-align: center;
    }
    
    .header-info {
        align-items: center;
    }
}
`

const dashboardJS = `
class PickboxDashboard {
    constructor() {
        this.updateInterval = 5000; // 5 seconds
        this.init();
    }

    init() {
        this.updateMetrics();
        setInterval(() => this.updateMetrics(), this.updateInterval);
    }

    async updateMetrics() {
        try {
            const [clusterData] = await Promise.all([
                fetch('/api/cluster').then(r => r.json())
            ]);

            this.updateClusterHealth(clusterData.health);
            this.updateReplicationMetrics(clusterData.metrics);
            this.updateSystemResources(clusterData.metrics);
            this.updateClusterStatus(clusterData);
        } catch (error) {
            console.error('Failed to update metrics:', error);
            this.showError();
        }
    }

    updateClusterHealth(health) {
        document.getElementById('node-state').textContent = health.state;
        document.getElementById('leader-node').textContent = health.leader || 'None';
        document.getElementById('peer-count').textContent = health.peers.length;
        document.getElementById('log-index').textContent = health.last_log_index.toLocaleString();

        // Update node state color
        const stateElement = document.getElementById('node-state');
        stateElement.className = 'metric-value';
        if (health.state === 'Leader') {
            stateElement.style.color = '#28a745';
        } else if (health.state === 'Follower') {
            stateElement.style.color = '#007bff';
        } else {
            stateElement.style.color = '#ffc107';
        }
    }

    updateReplicationMetrics(metrics) {
        document.getElementById('files-replicated').textContent = metrics.files_replicated.toLocaleString();
        document.getElementById('bytes-replicated').textContent = this.formatBytes(metrics.bytes_replicated);
        document.getElementById('avg-repl-time').textContent = metrics.avg_replication_time_ms + 'ms';
        
        const errorRate = metrics.replication_errors / Math.max(metrics.files_replicated, 1) * 100;
        document.getElementById('error-rate').textContent = errorRate.toFixed(2) + '%';
    }

    updateSystemResources(metrics) {
        document.getElementById('memory-usage').textContent = this.formatBytes(metrics.memory_usage_bytes);
        document.getElementById('goroutines').textContent = metrics.goroutines;
        document.getElementById('uptime').textContent = metrics.uptime;
        document.getElementById('last-replication').textContent = 
            metrics.last_replication_time === 'never' ? 'Never' : 
            new Date(metrics.last_replication_time).toLocaleTimeString();
    }

    updateClusterStatus(clusterData) {
        const statusLight = document.getElementById('status-light');
        const statusText = document.getElementById('status-text');
        const peerList = document.getElementById('peer-list');

        // Determine cluster health
        const isHealthy = clusterData.health.state === 'Leader' || clusterData.health.state === 'Follower';
        
        statusLight.className = 'status-light ' + (isHealthy ? 'healthy' : 'unhealthy');
        statusText.textContent = isHealthy ? 
            'Cluster is healthy and operational' : 
            'Cluster requires attention - check connectivity';

        // Update peer list
        peerList.innerHTML = '';
        
        // Add current node
        const currentNode = document.createElement('div');
        currentNode.className = 'peer-item';
        currentNode.innerHTML = ` +
	`<strong>${clusterData.metrics.node_id}</strong> (Current)
            <br>State: ${clusterData.health.state} +
            <br>Role: ${clusterData.health.state === 'Leader' ? '👑 Leader' : '👥 Follower'}` +
	`;
        peerList.appendChild(currentNode);

        // Add peer nodes
        clusterData.health.peers.forEach(peer => {
            const peerNode = document.createElement('div');
            peerNode.className = 'peer-item';
            peerNode.innerHTML = ` +
	`<strong>${peer}</strong> +
                <br>Status: Connected +
                <br>Role: Peer Node` +
	`;
            peerList.appendChild(peerNode);
        });
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    showError() {
        const statusLight = document.getElementById('status-light');
        const statusText = document.getElementById('status-text');
        
        statusLight.className = 'status-light unhealthy';
        statusText.textContent = 'Failed to fetch cluster metrics';
    }
}

// Initialize dashboard when page loads
document.addEventListener('DOMContentLoaded', () => {
    new PickboxDashboard();
});
`
