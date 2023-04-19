package record

import (
	"golang.org/x/time/rate"
	"sync"
	"time"
)

type inferLimiterStruct struct {
	Limiter  *rate.Limiter
	sync.Map // key: timestamp, value: InferPostStats
}

type InferPostStats struct {
	Success bool
	time.Time
}

var inferLimiter = inferLimiterStruct{
	Limiter: rate.NewLimiter(40, 60),
}

func (i *inferLimiterStruct) Allow() bool {
	if i.Limiter != nil && !i.Limiter.Allow() {
		return false
	}
	var success, failure int
	i.Range(func(key, value interface{}) bool {
		inferValue, ok := value.(InferPostStats)
		if !ok {
			return false
		}
		if inferValue.Before(time.Now().Add(-30 * time.Second)) {
			i.Delete(key)
			return true
		}
		if inferValue.Success {
			success++
		} else {
			failure++
		}
		return true
	})
	if failure > 10 && float64(failure)/float64(success+failure) > 0.5 {
		return false
	}
	return true
}

func (i *inferLimiterStruct) AddStats(success bool) {
	i.Store(time.Now().UnixNano(), InferPostStats{
		Success: success,
		Time:    time.Now(),
	})
}
