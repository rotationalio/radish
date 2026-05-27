package jitter

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

var (
	ErrInvalidMeanInterval = errors.New("mean interval duration must be positive")
	ErrInvalidSigma        = errors.New("standard deviation must create a positive distribution range")
)

type Ticker struct {
	C      <-chan time.Time
	cancel context.CancelFunc
}

func New(𝛍, 𝛔 time.Duration) (ticker *Ticker) {
	var err error
	if ticker, err = NewWithContext(context.Background(), 𝛍, 𝛔); err != nil {
		panic(err)
	}
	return ticker
}

func NewWithContext(ctx context.Context, 𝛍, 𝛔 time.Duration) (_ *Ticker, err error) {
	// Panic early if the distribution is invalid.
	if err = Check(𝛍, 𝛔); err != nil {
		return nil, err
	}

	// Add an internal cancel function to context to be stored for use in Stop.
	ctx, cancel := context.WithCancel(ctx)

	// Give the channel a 1 element time buffer.
	c := make(chan time.Time, 1)

	// Start the ticker in a goroutine.
	go func(ctx context.Context, c chan<- time.Time) {
		timer := time.NewTimer(Interval(𝛍, 𝛔)) // initial timer

		for {
			select {
			case tc := <-timer.C:
				// Reset the internal timer for the next duration.
				timer.Reset(Interval(𝛍, 𝛔))

				// Non-blocking rebroadcast of the tick
				select {
				case c <- tc:
				default:
				}
			case <-ctx.Done():
				// Stop the timer and drain its channel if needed.
				if !timer.Stop() {
					<-timer.C
				}

				// Exit the goroutine.
				return
			}
		}
	}(ctx, c)

	return &Ticker{C: c, cancel: cancel}, nil
}

// Stop turns off a ticker. After Stop, no more ticks will be sent.
// Stop does not close the channel, to prevent a concurrent goroutine reading from the
// channel from seeing an "tick" when it was the closing of the ticker.
func (t *Ticker) Stop() {
	t.cancel()
}

// Returns a random duration between 𝛍 - 3𝛔 and 𝛍 + 3𝛔.
func Interval(𝛍 time.Duration, 𝛔 time.Duration) time.Duration {
	// Try 8 times to get a positive sample.
	for range 8 {
		if sample := time.Duration(rand.NormFloat64()*float64(𝛔)) + 𝛍; sample > 0 {
			return sample
		}
	}

	// Fallback is to return the mean interval.
	return 𝛍
}

// Check ensures that a positive distribution range can be created from the mean and
// standard deviation in most cases.
func Check(𝛍 time.Duration, 𝛔 time.Duration) error {
	if 𝛍 <= 0 {
		return ErrInvalidMeanInterval
	}

	if 𝛔 <= 0 {
		return ErrInvalidSigma
	}

	// Capture 99.7% of the data (near total range)
	if 𝛍-(3*𝛔) <= 0 {
		return ErrInvalidSigma
	}

	return nil
}
