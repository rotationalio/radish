package radish

import (
	"github.com/kansaslabs/radish/api"
)

// Error codes that are common to the radish server
const (
	ErrUnknown int32 = iota
	ErrInvalidConfig
	ErrTaskAlreadyRegistered
	ErrTaskNotRegistered
	ErrNoWorkers
	ErrInvalidWorkers
)

// Errorf is a passthrough to api.Errorf, implemented here to allow for radish.Errorf calls.
func Errorf(code int32, format string, a ...interface{}) error {
	return api.Errorf(code, format, a...)
}
