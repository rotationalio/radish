package jitter_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish/jitter"
)

func TestJitter(t *testing.T) {
	var wg sync.WaitGroup

	ticker := jitter.New(500*time.Millisecond, 20*time.Millisecond)
	stop := make(chan struct{})

	wg.Add(1)
	time.AfterFunc(3*time.Second, func() {
		defer wg.Done()
		close(stop)
	})

	wg.Add(1)
	intervals := make([]time.Time, 0, 256)
	start := time.Now()
	go func() {
		defer wg.Done()
		for {
			select {
			case tick := <-ticker.C:
				intervals = append(intervals, tick)
			case <-stop:
				return
			}
		}
	}()

	wg.Wait()

	require.Greater(t, len(intervals), 4)
	require.Less(t, len(intervals), 12)

	for _, tick := range intervals {
		delta := tick.Sub(start)
		fmt.Printf("%s\n", delta)
		start = tick
	}
}

func TestInterval(t *testing.T) {
	prev := time.Duration(0)
	for range 256 {
		sample := jitter.Interval(5*time.Second, 150*time.Millisecond)
		require.Greater(t, sample, 0*time.Second)
		require.Less(t, sample, 10*time.Second)
		require.NotEqual(t, prev, sample)
		prev = sample
	}
}

func TestCheck(t *testing.T) {
	require.NoError(t, jitter.Check(5*time.Second, 150*time.Millisecond))
	require.ErrorIs(t, jitter.Check(0*time.Second, 150*time.Millisecond), jitter.ErrInvalidMeanInterval)
	require.ErrorIs(t, jitter.Check(-10*time.Second, 150*time.Millisecond), jitter.ErrInvalidMeanInterval)
	require.ErrorIs(t, jitter.Check(5*time.Second, -150*time.Millisecond), jitter.ErrInvalidSigma)
	require.ErrorIs(t, jitter.Check(5*time.Second, 3*time.Second), jitter.ErrInvalidSigma)
}
