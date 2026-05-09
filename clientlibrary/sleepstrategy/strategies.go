/*
 * Copyright (c) 2018 VMware, Inc.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
 * associated documentation files (the "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or substantial
 * portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
 * NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 * WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package sleepstrategy

import (
	"time"

	"github.com/vmware/vmware-go-kcl-v2/clientlibrary/interfaces"
)

// --- Default ---

type defaultIdleSleepStrategy struct{}

func (s *defaultIdleSleepStrategy) SleepDuration(records, _ int, idleTimeMs int) time.Duration {
	if records == 0 {
		return time.Duration(idleTimeMs) * time.Millisecond
	}
	return 0
}

// DefaultIdleSleepStrategyFactory preserves the original KCL behaviour:
// sleep IdleTimeBetweenReadsInMillis only when GetRecords returns 0 records.
func DefaultIdleSleepStrategyFactory() interfaces.IdleSleepStrategy {
	return &defaultIdleSleepStrategy{}
}

// --- Linear ---

type linearSleepStrategy struct{}

func (s *linearSleepStrategy) SleepDuration(records, maxRecords int, idleTimeMs int) time.Duration {
	fillRatio := float64(records) / float64(maxRecords)
	ms := int64(float64(idleTimeMs) * (1.0 - fillRatio))
	return time.Duration(ms) * time.Millisecond
}

// LinearSleepStrategyFactory returns a factory that creates LinearSleepStrategy instances.
//
// sleep = IdleTime × (1 - fillRatio)  — stateless, immediate response to each read.
//
// Example (IdleTime=1000ms, MaxRecords=1000):
//
//	fill%  sleep
//	  0%   1000 ms
//	 10%    900 ms
//	 30%    700 ms
//	 50%    500 ms
//	 70%    300 ms
//	 90%    100 ms
//	100%      0 ms
func LinearSleepStrategyFactory() func() interfaces.IdleSleepStrategy {
	return func() interfaces.IdleSleepStrategy { return &linearSleepStrategy{} }
}

// --- EMA ---

type emaSleepStrategy struct {
	alpha        float64
	fillRatioEMA float64
}

func (s *emaSleepStrategy) SleepDuration(records, maxRecords int, idleTimeMs int) time.Duration {
	currFillRatio := float64(records) / float64(maxRecords)
	s.fillRatioEMA = s.alpha*currFillRatio + (1-s.alpha)*s.fillRatioEMA
	ms := int64(float64(idleTimeMs) * (1.0 - s.fillRatioEMA))
	return time.Duration(ms) * time.Millisecond
}

// EMASleepStrategyFactory returns a factory that creates EMASleepStrategy instances.
//
// sleep = IdleTime × (1 - EMA)   where EMA = α×fill + (1-α)×prevEMA, init EMA=1.0
//
// Consecutive under-full reads ramp up sleep gradually; recovery to full batch
// shrinks sleep smoothly. Steady state equals LinearSleepStrategy.
// Recommended alpha: 0.3–0.5. Higher α reacts faster to stream changes.
//
// Example (IdleTime=1000ms, MaxRecords=1000, α=0.5):
//
//	Ramp-up: stream drops to fill=10% (EMA starts at 1.0)
//	read#  EMA    sleep
//	  1    0.55   450 ms
//	  2    0.33   675 ms
//	  3    0.21   788 ms
//	  5    0.13   871 ms
//	steady 0.10   900 ms  ← same as Linear at 10%
//
//	Recovery: stream returns to fill=100% (EMA starts at 0.10)
//	read#  EMA    sleep
//	  1    0.55   450 ms
//	  2    0.78   225 ms
//	  3    0.89   112 ms
//	  5    0.97    28 ms
//	  8    0.99    ~0 ms
func EMASleepStrategyFactory(alpha float64) func() interfaces.IdleSleepStrategy {
	return func() interfaces.IdleSleepStrategy {
		return &emaSleepStrategy{alpha: alpha, fillRatioEMA: 1.0}
	}
}
