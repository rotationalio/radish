package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"go.rtnl.ai/radish/jitter"
)

func Simulate(ctx context.Context, opts Simulator) (err error) {
	if err = opts.Validate(); err != nil {
		return err
	}

	if opts.StartDelay > 0 {
		time.Sleep(time.Duration(opts.StartDelay))
	}

	ticker := jitter.New(time.Duration(opts.Interval), time.Duration(opts.Sigma))
	defer ticker.Stop()

	if opts.StopWhen > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.StopWhen))
		defer cancel()
	}

	iter, enqueued := 0, 0
	for {
		select {
		case <-ticker.C:
			iter++
			n := opts.TasksPerInterval.Int()

			for i := 0; i < n; i++ {
				task := &Basic{
					Publisher: name,
					Delay:     opts.DelayRange.Duration(),
					ErrorProb: opts.ErrorProbability.Float64(),
					PanicProb: opts.PanicProbability.Float64(),
					FatalProb: opts.FatalProbability,
				}

				var id int64
				if rand.Float64() < opts.ScheduleProbability {
					executeAfter := time.Now().Add(opts.ScheduleRange.Duration())
					if id, err = tasks.Schedule(ctx, task, executeAfter); err != nil {
						return err
					}
					logger.Info("scheduled task", "id", id, "iter", iter)
				} else {
					if id, err = tasks.Enqueue(ctx, task); err != nil {
						return err
					}
					logger.Info("enqueued task", "id", id, "iter", iter)
				}
			}

			enqueued += n
			if opts.StopAfter > 0 && enqueued >= opts.StopAfter {
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// Options for the simulator.
type Simulator struct {
	Interval            Duration      `json:"interval"`
	Sigma               Duration      `json:"sigma"`
	TasksPerInterval    IntRange      `json:"tasks_per_interval"`
	ErrorProbability    FloatRange    `json:"error_probability"`
	PanicProbability    FloatRange    `json:"panic_probability"`
	DelayRange          DurationRange `json:"delay_range"`
	ScheduleProbability float64       `json:"schedule_probability"`
	ScheduleRange       DurationRange `json:"schedule_range"`
	FatalProbability    float64       `json:"fatal_probability"`
	StopAfter           int           `json:"stop_after,omitempty"`
	StopWhen            Duration      `json:"stop_when,omitempty"`
	StartDelay          Duration      `json:"start_delay,omitempty"`
}

type IntRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type FloatRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type DurationRange struct {
	Min Duration `json:"min"`
	Max Duration `json:"max"`
}

func (r *Simulator) Validate() error {
	if r.Interval <= 0 {
		return errors.New("interval must be greater than 0")
	}
	if r.Sigma <= 0 {
		return errors.New("sigma must be greater than 0")
	}

	if r.Interval-(3*r.Sigma) <= 0 {
		return errors.New("interval must be greater than 3 sigma")
	}

	if r.TasksPerInterval.Min < 1 {
		return errors.New("tasks per interval min must be greater than 1")
	}

	if r.DelayRange.Min <= 100*Duration(time.Millisecond) {
		return errors.New("delay range min must be greater than 100 milliseconds")
	}

	if r.ScheduleProbability > 0 {
		if r.ScheduleRange.Min <= 100*Duration(time.Millisecond) {
			return errors.New("schedule range min must be greater than 100 milliseconds")
		}
	}

	if r.TasksPerInterval.Min > r.TasksPerInterval.Max {
		return errors.New("tasks per interval min must be less than max")
	}
	if r.ErrorProbability.Min > r.ErrorProbability.Max {
		return errors.New("error probability min must be less than max")
	}
	if r.PanicProbability.Min > r.PanicProbability.Max {
		return errors.New("panic probability min must be less than max")
	}
	if r.DelayRange.Min > r.DelayRange.Max {
		return errors.New("delay range min must be less than max")
	}
	if r.ScheduleRange.Min > r.ScheduleRange.Max {
		return errors.New("schedule range min must be less than max")
	}

	if r.ScheduleProbability < 0 || r.ScheduleProbability > 1 {
		return errors.New("schedule probability must be between 0 and 1")
	}

	if r.FatalProbability < 0 || r.FatalProbability > 1 {
		return errors.New("fatal probability must be between 0 and 1")
	}

	if r.StopAfter <= 0 && r.StopWhen <= 0 {
		return errors.New("stop after or stop when must be provided")
	}

	return nil
}

func (r IntRange) Int() int {
	if r.Min == r.Max {
		return r.Min
	}
	return rand.IntN(r.Max-r.Min) + r.Min
}

func (r FloatRange) Float64() float64 {
	if r.Min == r.Max {
		return r.Min
	}
	return rand.Float64()*(r.Max-r.Min) + r.Min
}

func (r DurationRange) Duration() time.Duration {
	if r.Min == r.Max {
		return time.Duration(r.Min)
	}
	return time.Duration(rand.IntN(int(r.Max-r.Min))) + time.Duration(r.Min)
}

type Duration time.Duration

// MarshalJSON converts the duration to a quoted string (e.g., "5m30s")
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON parses the duration from a quoted string
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case string:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(duration)
		return nil
	default:
		return errors.New("invalid duration type")
	}
}
