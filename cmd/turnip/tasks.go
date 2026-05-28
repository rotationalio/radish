package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"time"

	"go.rtnl.ai/radish"
	"go.rtnl.ai/x/rlog"
)

var (
	ErrWheelSpin  = errors.New("the wheel of fate has spun and landed on error")
	ErrSprockets  = errors.New("the sprockets are misaligned and need greasing")
	ErrTonerOut   = errors.New("the toner is out and needs replacing")
	ErrDestiny    = errors.New("you walk along an incorrect path and are lost in the woods")
	ErrUnexpected = errors.New("an unexpected error occurred")
	ErrClosed     = errors.New("socket closed before writing finished")
	ErrAuthorized = errors.New("you are not authorized to perform this action")
	ErrExpired    = errors.New("the request has expired and is no longer valid")
	ErrConflict   = errors.New("the request conflicts with the current state of the resource")
	ErrAngst      = errors.New("you are feeling the angst of the universe and it is overwhelming you")
)

var PossibleErrors = []error{
	ErrWheelSpin,
	ErrSprockets,
	ErrTonerOut,
	ErrDestiny,
	ErrUnexpected,
	ErrClosed,
	ErrAuthorized,
	ErrExpired,
	ErrConflict,
	ErrAngst,
}

type Basic struct {
	Publisher string        `json:"publisher,omitempty"`
	Delay     time.Duration `json:"delay,omitempty"`
	ErrorProb float64       `json:"error_prob,omitempty"`
	PanicProb float64       `json:"panic_prob,omitempty"`
	FatalProb float64       `json:"fatal_prob,omitempty"`
}

func (t *Basic) Kind() string {
	return "basic"
}

func Default(ctx context.Context, task *radish.TaskInfo[*Basic]) error {
	time.Sleep(task.Task.Delay)
	if rand.Float64() < task.Task.ErrorProb {
		return PossibleErrors[rand.IntN(len(PossibleErrors))]
	}
	if rand.Float64() < task.Task.PanicProb {
		if task.Task.FatalProb > 0 && rand.Float64() < task.Task.FatalProb {
			rlog.Fatal("task fatal error", "pub", task.Task.Publisher, "sub", name)
			os.Exit(1)
		}
		panic(PossibleErrors[rand.IntN(len(PossibleErrors))])
	}
	logger.Info("task completed", "pub", task.Task.Publisher, "sub", name)
	return nil
}
