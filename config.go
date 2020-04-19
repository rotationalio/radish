package radish

import (
	"runtime"
	"strings"

	"github.com/kansaslabs/x/out"
)

var logLevels = map[string]uint8{
	"trace":  out.LevelTrace,
	"debug":  out.LevelDebug,
	"info":   out.LevelInfo,
	"status": out.LevelStatus,
	"warn":   out.LevelWarn,
	"silent": out.LevelSilent,
}

// Config allows you to specify runtime options to the Radish server and job queue.
type Config struct {
	Workers          int    // the number of workers to start radish with (default is num cpus)
	LogLevel         string // the level to log at (default is info)
	CautionThreshold uint   // the number of messages accumulated before issuing another caution
}

// Validate the config and populate any defaults for zero valued configurations
func (c *Config) Validate() (err error) {
	// Handle the number of workers
	if c.Workers <= 0 {
		c.Workers = runtime.NumCPU()
	}

	// Handle the log level
	if c.LogLevel == "" {
		c.LogLevel = "info"
	} else {
		c.LogLevel = strings.ToLower(c.LogLevel)
		if _, ok := logLevels[c.LogLevel]; !ok {
			return Errorf(ErrInvalidConfig, "%q is an invalid log level, use trace, debug, info, status, warn, or silent", c.LogLevel)
		}
	}

	// Handle the caution threshold
	if c.CautionThreshold == 0 {
		c.CautionThreshold = out.DefaultCautionThreshold
	}

	return nil
}

func (c *Config) setLogLevel() {
	out.SetLogLevel(logLevels[c.LogLevel])
}

func (c *Config) setCautionThreshold() {
	out.SetCautionThreshold(c.CautionThreshold)
}
