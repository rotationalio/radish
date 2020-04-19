package api

import "fmt"

// Errorf formats a radish error with the specified code, returning it.
func Errorf(code int32, format string, a ...interface{}) error {
	msg := fmt.Errorf(format, a...)
	return &Error{Code: code, Message: msg.Error()}
}

// Error implements the error interface
func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}
