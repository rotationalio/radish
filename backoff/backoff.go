package backoff

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

const (
	PolicyZero        = "zero"
	PolicyConstant    = "constant"
	PolicyLinear      = "linear"
	PolicyExponential = "exponential"
)

func New(cfg Config) (backoff BackOff, err error) {
	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.Policy {
	case PolicyZero:
		return &ZeroBackOff{}, nil
	case PolicyConstant:
		backoff = &ConstantBackOff{delay: cfg.Delay}
	case PolicyLinear:
		backoff = &LinearBackOff{delay: cfg.Delay}
	case PolicyExponential:
		backoff = &ExponentialBackOff{delay: cfg.Delay, multiplier: cfg.Factor}
	default:
		return nil, fmt.Errorf("unimplemented policy: %q", cfg.Policy)
	}

	if cfg.Jitter {
		backoff = &JitterBackOff{backoff: backoff, sigma: float64(cfg.Sigma)}
	}
	return backoff, nil
}

// Backoff is an interface for delaying the next execution of a task to give the system
// time to recover from errors. The next delay is determined by the number of attempts
// and different backoff policies might compute the delay differently.
type BackOff interface {
	Delay(attempts int) time.Duration
}

// ZeroBackOff is a backoff policy that never delays the next execution of a task.
type ZeroBackOff struct{}

func (b *ZeroBackOff) Delay(attempts int) time.Duration {
	return 0
}

// JitterBackOff is a backoff policy that adds a random jitter to the delay provided by
// the underlying backoff policy. Randomness is normally distributed about the mean
// delay specified by the returned delay along with a standard deviation specified by
// the sigma parameter. If the returned result is less than 0, it is multiplied by -1
// to make it positive.
type JitterBackOff struct {
	backoff BackOff
	sigma   float64
}

func (b *JitterBackOff) Delay(attempts int) time.Duration {
	delay := b.backoff.Delay(attempts)
	delay = time.Duration(rand.NormFloat64()*b.sigma) + delay
	if delay < 0 {
		delay = -delay
	}
	return delay
}

// ConstantBackOff is a backoff policy that delays the next execution of a task by a
// constant amount. This is the simplest backoff policy and is generally used for testing.
type ConstantBackOff struct {
	delay time.Duration
}

func (b *ConstantBackOff) Delay(attempts int) time.Duration {
	return b.delay
}

// LinearBackOff is a backoff policy that delays the next execution of a task by a
// an increasing amount determined by the multiplier. The initial delay is the is the
// intercept of the linear equation and the attempt is x and the multiplier is the slope.
type LinearBackOff struct {
	delay time.Duration
}

func (b *LinearBackOff) Delay(attempts int) time.Duration {
	return time.Duration(attempts) * b.delay
}

// ExponentialBackOff is a backoff policy that delays the next execution of a task by a
// an increasing amount determined by the multiplier.
type ExponentialBackOff struct {
	delay      time.Duration
	multiplier float64
}

func (b *ExponentialBackOff) Delay(attempts int) time.Duration {
	return time.Duration(math.Pow(b.multiplier, float64(attempts))) * b.delay
}
