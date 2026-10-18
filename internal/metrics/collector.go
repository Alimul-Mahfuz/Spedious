package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/spedious/spedious/internal/protocol"
)

// MetricStats represents percentile and summary stats for a timing metric.
type MetricStats struct {
	Count int64   `json:"count"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Mean  float64 `json:"mean"`
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	P999  float64 `json:"p999"`
}

// TimeSeriesSample captures a periodic data point for time-series export.
type TimeSeriesSample struct {
	Timestamp   time.Time `json:"timestamp"`
	ElapsedSec  float64   `json:"elapsed_sec"`
	ActiveVUs   int64     `json:"active_vus"`
	RPS         float64   `json:"rps"`
	Errors      int64     `json:"errors"`
	ErrorRate   float64   `json:"error_rate"`
	P95Duration float64   `json:"p95_duration"`
}

// Snapshot captures point-in-time metrics summary.
type Snapshot struct {
	TotalRequests     int64         `json:"total_requests"`
	TotalErrors       int64         `json:"total_errors"`
	TotalIterations   int64         `json:"total_iterations"`
	ActiveVUs         int64         `json:"active_vus"`
	BytesSent         int64         `json:"bytes_sent"`
	BytesReceived     int64         `json:"bytes_received"`
	DroppedIterations int64         `json:"dropped_iterations"`
	Duration          time.Duration `json:"duration"`

	// Overall http_req_duration percentiles (in milliseconds)
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	P999  float64 `json:"p999"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Mean  float64 `json:"mean"`
	Count int64   `json:"count"`

	// Timing breakdown metrics
	BlockedDuration    MetricStats `json:"blocked"`
	ConnectingDuration MetricStats `json:"connecting"`
	TLSDuration        MetricStats `json:"tls"`
	WaitingDuration    MetricStats `json:"waiting"`
	ReceivingDuration  MetricStats `json:"receiving"`
	IterationDuration  MetricStats `json:"iteration_duration"`

	// Per-step breakdown
	StepStats map[string]StepSnapshot `json:"steps"`

	// Time series history
	TimeSeries []TimeSeriesSample `json:"time_series,omitempty"`
}

// StepSnapshot captures metrics for a specific step or request.
type StepSnapshot struct {
	Name     string  `json:"name"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	P99      float64 `json:"p99"`
	Mean     float64 `json:"mean"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// Collector records metrics thread-safely across all VUs.
type Collector struct {
	mu sync.RWMutex

	startTime time.Time

	totalRequests     atomic.Int64
	totalErrors       atomic.Int64
	totalIterations   atomic.Int64
	activeVUs         atomic.Int64
	bytesSent         atomic.Int64
	bytesReceived     atomic.Int64
	droppedIterations atomic.Int64

	durationHist   *hdrhistogram.Histogram
	blockedHist    *hdrhistogram.Histogram
	connectingHist *hdrhistogram.Histogram
	tlsHist        *hdrhistogram.Histogram
	waitingHist    *hdrhistogram.Histogram
	receivingHist  *hdrhistogram.Histogram
	iterationHist  *hdrhistogram.Histogram

	stepHists map[string]*hdrhistogram.Histogram
	stepReqs  map[string]*atomic.Int64
	stepErrs  map[string]*atomic.Int64

	timeSeries []TimeSeriesSample
	lastReqs   int64
	lastTime   time.Time
}

// NewCollector creates an initialized metrics collector.
func NewCollector() *Collector {
	newHist := func() *hdrhistogram.Histogram {
		return hdrhistogram.New(1, 3600000000, 3) // 1 us to 1 hour
	}

	now := time.Now()
	return &Collector{
		startTime:      now,
		lastTime:       now,
		durationHist:   newHist(),
		blockedHist:    newHist(),
		connectingHist: newHist(),
		tlsHist:        newHist(),
		waitingHist:    newHist(),
		receivingHist:  newHist(),
		iterationHist:  newHist(),
		stepHists:      make(map[string]*hdrhistogram.Histogram),
		stepReqs:       make(map[string]*atomic.Int64),
		stepErrs:       make(map[string]*atomic.Int64),
		timeSeries:     make([]TimeSeriesSample, 0, 100),
	}
}

// Reset resets the start time and clears all metrics.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.startTime = now
	c.lastTime = now
	c.lastReqs = 0
	c.totalRequests.Store(0)
	c.totalErrors.Store(0)
	c.totalIterations.Store(0)
	c.activeVUs.Store(0)
	c.bytesSent.Store(0)
	c.bytesReceived.Store(0)
	c.droppedIterations.Store(0)

	c.durationHist.Reset()
	c.blockedHist.Reset()
	c.connectingHist.Reset()
	c.tlsHist.Reset()
	c.waitingHist.Reset()
	c.receivingHist.Reset()
	c.iterationHist.Reset()

	c.stepHists = make(map[string]*hdrhistogram.Histogram)
	c.stepReqs = make(map[string]*atomic.Int64)
	c.stepErrs = make(map[string]*atomic.Int64)
	c.timeSeries = make([]TimeSeriesSample, 0, 100)
}

// SetActiveVUs updates the gauge for active virtual users.
func (c *Collector) SetActiveVUs(vus int64) {
	c.activeVUs.Store(vus)
}

// IncIterations increments the total completed iterations count.
func (c *Collector) IncIterations() {
	c.totalIterations.Add(1)
}

// RecordIterationDuration records the full duration of a completed iteration.
func (c *Collector) RecordIterationDuration(d time.Duration) {
	micros := d.Microseconds()
	if micros < 1 {
		micros = 1
	}
	c.mu.Lock()
	_ = c.iterationHist.RecordValue(micros)
	c.mu.Unlock()
}

// IncDroppedIterations increments the dropped iterations count.
func (c *Collector) IncDroppedIterations() {
	c.droppedIterations.Add(1)
}

// RecordRequest records a completed request with its full timing breakdown.
func (c *Collector) RecordRequest(stepName string, duration time.Duration, isError bool, sent, recv int64) {
	c.RecordRequestWithTiming(stepName, protocol.RequestTiming{TotalDuration: duration}, isError, sent, recv)
}

// RecordRequestWithTiming records a completed request with its full httptrace timing breakdown.
func (c *Collector) RecordRequestWithTiming(stepName string, timing protocol.RequestTiming, isError bool, sent, recv int64) {
	c.totalRequests.Add(1)
	if isError {
		c.totalErrors.Add(1)
	}
	c.bytesSent.Add(sent)
	c.bytesReceived.Add(recv)

	micros := timing.TotalDuration.Microseconds()
	if micros < 1 {
		micros = 1
	}

	c.mu.Lock()
	_ = c.durationHist.RecordValue(micros)

	if timing.Blocked > 0 {
		_ = c.blockedHist.RecordValue(timing.Blocked.Microseconds())
	}
	if timing.Connecting > 0 {
		_ = c.connectingHist.RecordValue(timing.Connecting.Microseconds())
	}
	if timing.TLSHandshake > 0 {
		_ = c.tlsHist.RecordValue(timing.TLSHandshake.Microseconds())
	}
	if timing.Waiting > 0 {
		_ = c.waitingHist.RecordValue(timing.Waiting.Microseconds())
	}
	if timing.Receiving > 0 {
		_ = c.receivingHist.RecordValue(timing.Receiving.Microseconds())
	}

	stepHist, ok := c.stepHists[stepName]
	if !ok {
		stepHist = hdrhistogram.New(1, 3600000000, 3)
		c.stepHists[stepName] = stepHist
		c.stepReqs[stepName] = new(atomic.Int64)
		c.stepErrs[stepName] = new(atomic.Int64)
	}
	_ = stepHist.RecordValue(micros)
	c.mu.Unlock()

	c.stepReqs[stepName].Add(1)
	if isError {
		c.stepErrs[stepName].Add(1)
	}
}

// RecordSample records a time-series sample for live graphs or CSV exports.
func (c *Collector) RecordSample() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(c.startTime).Seconds()
	intervalSec := now.Sub(c.lastTime).Seconds()
	totalReq := c.totalRequests.Load()
	totalErr := c.totalErrors.Load()

	rps := 0.0
	if intervalSec > 0 {
		rps = float64(totalReq-c.lastReqs) / intervalSec
	}
	c.lastReqs = totalReq
	c.lastTime = now

	errRate := 0.0
	if totalReq > 0 {
		errRate = float64(totalErr) / float64(totalReq)
	}

	p95 := float64(c.durationHist.ValueAtQuantile(95)) / 1000.0

	c.timeSeries = append(c.timeSeries, TimeSeriesSample{
		Timestamp:   now,
		ElapsedSec:  elapsed,
		ActiveVUs:   c.activeVUs.Load(),
		RPS:         rps,
		Errors:      totalErr,
		ErrorRate:   errRate,
		P95Duration: p95,
	})
}

// TimeSeries returns a copy of recorded time-series samples.
func (c *Collector) TimeSeries() []TimeSeriesSample {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]TimeSeriesSample, len(c.timeSeries))
	copy(res, c.timeSeries)
	return res
}

// Snapshot computes and returns the current aggregate metrics snapshot.
func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalReq := c.totalRequests.Load()
	totalErr := c.totalErrors.Load()
	totalIter := c.totalIterations.Load()
	activeVU := c.activeVUs.Load()
	sent := c.bytesSent.Load()
	recv := c.bytesReceived.Load()
	dropped := c.droppedIterations.Load()

	dur := time.Since(c.startTime)

	s := Snapshot{
		TotalRequests:      totalReq,
		TotalErrors:        totalErr,
		TotalIterations:    totalIter,
		ActiveVUs:          activeVU,
		BytesSent:          sent,
		BytesReceived:      recv,
		DroppedIterations:  dropped,
		Duration:           dur,
		Count:              c.durationHist.TotalCount(),
		P50:                float64(c.durationHist.ValueAtQuantile(50)) / 1000.0,
		P90:                float64(c.durationHist.ValueAtQuantile(90)) / 1000.0,
		P95:                float64(c.durationHist.ValueAtQuantile(95)) / 1000.0,
		P99:                float64(c.durationHist.ValueAtQuantile(99)) / 1000.0,
		P999:               float64(c.durationHist.ValueAtQuantile(99.9)) / 1000.0,
		Min:                float64(c.durationHist.Min()) / 1000.0,
		Max:                float64(c.durationHist.Max()) / 1000.0,
		Mean:               c.durationHist.Mean() / 1000.0,
		BlockedDuration:    toMetricStats(c.blockedHist),
		ConnectingDuration: toMetricStats(c.connectingHist),
		TLSDuration:        toMetricStats(c.tlsHist),
		WaitingDuration:    toMetricStats(c.waitingHist),
		ReceivingDuration:  toMetricStats(c.receivingHist),
		IterationDuration:  toMetricStats(c.iterationHist),
		StepStats:          make(map[string]StepSnapshot, len(c.stepHists)),
		TimeSeries:         make([]TimeSeriesSample, len(c.timeSeries)),
	}

	copy(s.TimeSeries, c.timeSeries)

	for name, hist := range c.stepHists {
		var reqs int64
		if r, ok := c.stepReqs[name]; ok {
			reqs = r.Load()
		}
		var errs int64
		if e, ok := c.stepErrs[name]; ok {
			errs = e.Load()
		}

		s.StepStats[name] = StepSnapshot{
			Name:     name,
			Requests: reqs,
			Errors:   errs,
			P50:      float64(hist.ValueAtQuantile(50)) / 1000.0,
			P95:      float64(hist.ValueAtQuantile(95)) / 1000.0,
			P99:      float64(hist.ValueAtQuantile(99)) / 1000.0,
			Mean:     hist.Mean() / 1000.0,
			Min:      float64(hist.Min()) / 1000.0,
			Max:      float64(hist.Max()) / 1000.0,
		}
	}

	return s
}

func toMetricStats(h *hdrhistogram.Histogram) MetricStats {
	if h.TotalCount() == 0 {
		return MetricStats{}
	}
	return MetricStats{
		Count: h.TotalCount(),
		Min:   float64(h.Min()) / 1000.0,
		Max:   float64(h.Max()) / 1000.0,
		Mean:  h.Mean() / 1000.0,
		P50:   float64(h.ValueAtQuantile(50)) / 1000.0,
		P90:   float64(h.ValueAtQuantile(90)) / 1000.0,
		P95:   float64(h.ValueAtQuantile(95)) / 1000.0,
		P99:   float64(h.ValueAtQuantile(99)) / 1000.0,
		P999:  float64(h.ValueAtQuantile(99.9)) / 1000.0,
	}
}
