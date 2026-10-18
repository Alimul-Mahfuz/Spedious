package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCollector_RecordAndSnapshot(t *testing.T) {
	c := NewCollector()
	c.SetActiveVUs(5)

	c.RecordRequest("get_items", 10*time.Millisecond, false, 100, 500)
	c.RecordRequest("get_items", 20*time.Millisecond, false, 100, 500)
	c.RecordRequest("post_item", 50*time.Millisecond, true, 200, 50)
	c.IncIterations()

	snap := c.Snapshot()
	assert.Equal(t, int64(3), snap.TotalRequests)
	assert.Equal(t, int64(1), snap.TotalErrors)
	assert.Equal(t, int64(1), snap.TotalIterations)
	assert.Equal(t, int64(5), snap.ActiveVUs)
	assert.Equal(t, int64(400), snap.BytesSent)
	assert.Equal(t, int64(1050), snap.BytesReceived)

	assert.True(t, snap.Min >= 10.0)
	assert.True(t, snap.Max >= 50.0)

	getItemStat, ok := snap.StepStats["get_items"]
	assert.True(t, ok)
	assert.Equal(t, int64(2), getItemStat.Requests)
	assert.Equal(t, int64(0), getItemStat.Errors)
}

func TestCollector_Concurrent(t *testing.T) {
	c := NewCollector()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.RecordRequest("req", time.Duration(j+1)*time.Millisecond, false, 50, 100)
			}
		}(i)
	}
	wg.Wait()

	snap := c.Snapshot()
	assert.Equal(t, int64(5000), snap.TotalRequests)
}
