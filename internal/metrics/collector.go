package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// Snapshot captures point-in-time metrics summary.
type Snapshot struct {
	TotalRequests     int64
	TotalErrors       int64
	TotalIterations   int64
	ActiveVUs         int64
	BytesSent         int64
	BytesReceived     int64
	DroppedIterations int64
	Duration          time.Duration

	// Percentiles in milliseconds
	P50   float64
	P90   float64
	P95   float64
	P99   float64
	P999  float64
	Min   float64
	Max   float64
	Mean  float64
	Count int64

	// Per-step breakdown
	StepStats map[string]StepSnapshot
}

// StepSnapshot captures metrics for a specific step or request.
type StepSnapshot struct {
	Name     string
	Requests int64
	Errors   int64
	P50      float64
	P95      float64
	P99      float64
	Mean     float64
	Min      float64
	Max      float64
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

	overallHist *hdrhistogram.Histogram
	stepHists   map[string]*hdrhistogram.Histogram
	stepReqs    map[string]*atomic.Int64
	stepErrs    map[string]*atomic.Int64
}

// NewCollector creates an initialized metrics collector.
func NewCollector() *Collector {
	// Histogram ranges from 1 microsecond to 1 hour (3,600,000,000 us) with 3 sig figs
	hist := hdrhistogram.New(1, 3600000000, 3)
	return &Collector{
		startTime:   time.Now(),
		overallHist: hist,
		stepHists:   make(map[string]*hdrhistogram.Histogram),
		stepReqs:    make(map[string]*atomic.Int64),
		stepErrs:    make(map[string]*atomic.Int64),
	}
}

// Reset resets the start time and clear metrics.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startTime = time.Now()
	c.totalRequests.Store(0)
	c.totalErrors.Store(0)
	c.totalIterations.Store(0)
	c.activeVUs.Store(0)
	c.bytesSent.Store(0)
	c.bytesReceived.Store(0)
	c.droppedIterations.Store(0)
	c.overallHist.Reset()
	c.stepHists = make(map[string]*hdrhistogram.Histogram)
	c.stepReqs = make(map[string]*atomic.Int64)
	c.stepErrs = make(map[string]*atomic.Int64)
}

// SetActiveVUs updates the gauge for active virtual users.
func (c *Collector) SetActiveVUs(vus int64) {
	c.activeVUs.Store(vus)
}

// IncIterations increments the total completed iterations count.
func (c *Collector) IncIterations() {
	c.totalIterations.Add(1)
}

// IncDroppedIterations increments the dropped iterations count.
func (c *Collector) IncDroppedIterations() {
	c.droppedIterations.Add(1)
}

// RecordRequest records a single completed request step.
func (c *Collector) RecordRequest(stepName string, duration time.Duration, isError bool, sent, recv int64) {
	c.totalRequests.Add(1)
	if isError {
		c.totalErrors.Add(1)
	}
	c.bytesSent.Add(sent)
	c.bytesReceived.Add(recv)

	// Record in microseconds
	micros := duration.Microseconds()
	if micros < 1 {
		micros = 1
	}

	c.mu.Lock()
	_ = c.overallHist.RecordValue(micros)

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
		TotalRequests:     totalReq,
		TotalErrors:       totalErr,
		TotalIterations:   totalIter,
		ActiveVUs:         activeVU,
		BytesSent:         sent,
		BytesReceived:     recv,
		DroppedIterations: dropped,
		Duration:          dur,
		Count:             c.overallHist.TotalCount(),
		P50:               float64(c.overallHist.ValueAtQuantile(50)) / 1000.0,
		P90:               float64(c.overallHist.ValueAtQuantile(90)) / 1000.0,
		P95:               float64(c.overallHist.ValueAtQuantile(95)) / 1000.0,
		P99:               float64(c.overallHist.ValueAtQuantile(99)) / 1000.0,
		P999:              float64(c.overallHist.ValueAtQuantile(99.9)) / 1000.0,
		Min:               float64(c.overallHist.Min()) / 1000.0,
		Max:               float64(c.overallHist.Max()) / 1000.0,
		Mean:              c.overallHist.Mean() / 1000.0,
		StepStats:         make(map[string]StepSnapshot, len(c.stepHists)),
	}

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
