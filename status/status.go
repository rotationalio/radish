package status

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

type Status uint8

const (
	Unknown Status = iota
	Pending
	Retry
	Scheduled
	Running
	Succeeded
	Failed
	Cancelled

	// The terminator is used to determine the last value of the enum.
	statusTerminator
)

var statusNames = [8]string{
	"unknown",
	"pending",
	"retry",
	"scheduled",
	"running",
	"succeeded",
	"failed",
	"cancelled",
}

func Parse(s any) (Status, error) {
	switch v := s.(type) {
	case string:
		s = strings.TrimSpace(strings.ToLower(v))
		if s == "" {
			return Unknown, nil
		}

		for i, name := range statusNames {
			if name == s {
				return Status(i), nil
			}
		}

		return Unknown, fmt.Errorf("invalid status: %q", s)
	case float64:
		if v < 0 || v > float64(^uint8(0)) || v != float64(uint8(v)) {
			return Unknown, fmt.Errorf("cannot parse %v to Status", v)
		}
		return Status(uint8(v)), nil
	case uint8:
		return Status(v), nil
	case Status:
		return v, nil
	default:
		return Unknown, fmt.Errorf("cannot parse %T to Status", s)
	}
}

func (s Status) String() string {
	if s >= statusTerminator {
		return statusNames[0]
	}
	return statusNames[s]
}

func (s Status) Uint8() uint8 {
	return uint8(s)
}

//============================================================================
// JSON Serialization and Deserialization
//============================================================================

func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Status) UnmarshalJSON(data []byte) (err error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	if *s, err = Parse(v); err != nil {
		return err
	}
	return nil
}

//===========================================================================
// Database Interaction
//===========================================================================

func (s *Status) Scan(src interface{}) (err error) {
	switch x := src.(type) {
	case nil:
		return nil
	case string:
		*s, err = Parse(x)
		return err
	case []byte:
		*s, err = Parse(string(x))
		return err
	default:
		return fmt.Errorf("cannot scan %T into a status", src)
	}
}

func (s Status) Value() (driver.Value, error) {
	return s.String(), nil
}
