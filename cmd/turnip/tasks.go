package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"go.rtnl.ai/radish"
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
	Delay     time.Duration `json:"delay,omitempty"`
	ErrorProb float64       `json:"error_prob,omitempty"`
}

func (t *Basic) Kind() string {
	return "basic"
}

func Default(ctx context.Context, task *radish.TaskInfo[*Basic]) error {
	time.Sleep(task.Task.Delay)
	if rand.Float64() < task.Task.ErrorProb {
		return PossibleErrors[rand.IntN(len(PossibleErrors))]
	}
	return nil
}
