package infra

import (
	"maps"
	"runtime"
	"sync"
	"time"
)

// Alert represents a system anomaly or heartbeat timeout notification.
type Alert struct {
	Level     string         `json:"level"`     // info, warn, critical
	Source    string         `json:"source"`    // component name or "alerter"
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Metrics   map[string]any `json:"metrics,omitempty"`
}

// AlertHandler receives alerts. Implementations can forward to log, webhook,
// PagerDuty,钉钉, 企业微信, etc.
type AlertHandler interface {
	Handle(alert Alert)
}

// LogAlertHandler writes alerts via a Logger.
type LogAlertHandler struct {
	logger Logger
}

// NewLogAlertHandler creates a handler that forwards alerts to the given logger.
func NewLogAlertHandler(l Logger) *LogAlertHandler {
	return &LogAlertHandler{logger: l}
}

// Handle implements AlertHandler.
func (h *LogAlertHandler) Handle(a Alert) {
	if h.logger == nil {
		return
	}
	fields := map[string]any{
		"source":  a.Source,
		"level":   a.Level,
		"message": a.Message,
	}
	maps.Copy(fields, a.Metrics)
	switch a.Level {
	case "critical":
		h.logger.Error("ALERT", fields)
	case "warn":
		h.logger.Warn("ALERT", fields)
	default:
		h.logger.Info("ALERT", fields)
	}
}

// Alerter is a dedicated low-CPU goroutine that monitors heartbeats and
// emits alerts when anomalies are detected.
//
// CPU-friendly design:
//   - ticker-driven (no busy loops)
//   - yields CPU via runtime.Gosched() every N checks
//   - all heavy work is offloaded to AlertHandler goroutines
type Alerter struct {
	registry      *HeartbeatRegistry
	checkInterval time.Duration
	alertCh       chan Alert
	handlers      []AlertHandler
	yieldEvery    int

	stopCh chan struct{}
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewAlerter creates an alerter. If registry is nil a default one is created.
func NewAlerter(registry *HeartbeatRegistry, checkInterval time.Duration) *Alerter {
	if registry == nil {
		registry = NewHeartbeatRegistry()
	}
	if checkInterval <= 0 {
		checkInterval = 30 * time.Second
	}
	return &Alerter{
		registry:      registry,
		checkInterval: checkInterval,
		alertCh:       make(chan Alert, 128),
		yieldEvery:    10,
		stopCh:        make(chan struct{}),
	}
}

// RegisterHandler adds an alert sink.
func (a *Alerter) RegisterHandler(h AlertHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handlers = append(a.handlers, h)
}

// Registry exposes the underlying heartbeat registry so other components
// can register themselves and call Beat().
func (a *Alerter) Registry() *HeartbeatRegistry {
	return a.registry
}

// Start launches the dedicated alerter goroutine.
func (a *Alerter) Start() {
	a.wg.Add(1)
	go a.loop()
}

// Stop shuts down the alerter gracefully.
func (a *Alerter) Stop() {
	close(a.stopCh)
	a.wg.Wait()
}

// Alert emits an alert immediately (non-blocking).
func (a *Alerter) Alert(level, source, message string) {
	select {
	case a.alertCh <- Alert{
		Level:     level,
		Source:    source,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}:
	default:
		// channel full, alert dropped to avoid blocking caller
	}
}

// AlertWithMetrics emits an alert with attached metrics.
func (a *Alerter) AlertWithMetrics(level, source, message string, metrics map[string]any) {
	select {
	case a.alertCh <- Alert{
		Level:     level,
		Source:    source,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metrics:   metrics,
	}:
	default:
	}
}

func (a *Alerter) loop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.checkInterval)
	defer ticker.Stop()

	checkCount := 0
	for {
		select {
		case <-ticker.C:
			a.checkHeartbeats()
			a.emitSelfHeartbeat()

			checkCount++
			if checkCount >= a.yieldEvery {
				checkCount = 0
				runtime.Gosched() // yield CPU to other goroutines (微观共享)
			}

		case alert := <-a.alertCh:
			a.dispatch(alert)

		case <-a.stopCh:
			return
		}
	}
}

func (a *Alerter) checkHeartbeats() {
	expired := a.registry.Check()
	if len(expired) == 0 {
		return
	}
	for _, id := range expired {
		a.Alert("critical", id, "heartbeat timeout: component may be dead")
	}
}

func (a *Alerter) emitSelfHeartbeat() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var sys runtime.MemStats
	runtime.ReadMemStats(&sys)

	// Self-monitoring: alert if this alerter itself is under pressure
	if sys.Sys > 2*1024*1024*1024 { // 2GB
		a.AlertWithMetrics("warn", "alerter", "high memory pressure", map[string]any{
			"sys_mb":  sys.Sys / 1024 / 1024,
			"heap_mb": sys.HeapSys / 1024 / 1024,
		})
	}
}

func (a *Alerter) dispatch(alert Alert) {
	a.mu.RLock()
	handlers := make([]AlertHandler, len(a.handlers))
	copy(handlers, a.handlers)
	a.mu.RUnlock()

	for _, h := range handlers {
		h.Handle(alert)
	}
}
