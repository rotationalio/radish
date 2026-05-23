package backoff

import (
	"time"

	"go.rtnl.ai/confire"
)

type Config struct {
	Policy string        `default:"linear" desc:"The policy to use for backoff, this determines what settings are required."`
	Delay  time.Duration `default:"10s" desc:"The delay to use for constant backoff or the initial delay for linear and exponential backoff."`
	Factor float64       `default:"2.0" desc:"The factor to use for exponential backoff."`
	Jitter bool          `default:"false" desc:"Add a random jitter to the delay to prevent thundering herds."`
	Sigma  time.Duration `default:"750ms" desc:"The standard deviation to use for jitter."`
}

func (c Config) Validate() (err error) {
	if c.Policy == "" {
		return confire.Required("backoff", "policy")
	}

	switch c.Policy {
	case PolicyZero:
		return nil
	case PolicyConstant:
		if c.Delay <= 0 {
			err = confire.Join(err, confire.Required("backoff", "delay"))
		}
	case PolicyLinear:
		if c.Delay <= 0 {
			err = confire.Join(err, confire.Required("backoff", "delay"))
		}
	case PolicyExponential:
		if c.Delay <= 0 {
			err = confire.Join(err, confire.Required("backoff", "delay"))
		}
		if c.Factor <= 0 {
			err = confire.Join(err, confire.Required("backoff", "factor"))
		}
	default:
		return confire.Invalid("backoff", "policy", "unknown policy: %s", c.Policy)
	}

	if c.Jitter {
		if c.Sigma <= 0 {
			err = confire.Join(err, confire.Required("backoff", "sigma"))
		}
	}

	return err
}
