package radish

import (
	"database/sql"
	"time"

	"go.rtnl.ai/confire"
)

const Prefix = "radish"

type Config struct {
	DatabaseURL  string        `env:"DATABASE_URL" desc:"DSN to the postgres database to use for task persistence."`
	ManagedDB    bool          `default:"false" split_words:"true" desc:"Do not connect to the database, use a provided connection instead."`
	NumWorkers   int           `default:"8" split_words:"true" desc:"The number of workers to use for concurrent task processing."`
	TaskRetries  int           `default:"3" split_words:"true" desc:"The number of times to retry a task if it fails."`
	TaskTimeout  time.Duration `default:"60s" split_words:"true" desc:"The amount of time for a task to complete before it is cancelled and requeued."`
	PollInterval time.Duration `default:"5s" split_words:"true" desc:"The interval to poll the database for new tasks."`
	PollJitter   time.Duration `default:"125ms" split_words:"true" desc:"The jitter to add to the poll interval to prevent thundering herds."`
	Retention    time.Duration `default:"24h" desc:"The duration to retain completed tasks in the database."`
	Conn         *sql.DB       `ignored:"true" desc:"Provide a database connection rather than allowing radish to connect itself."`
}

func LoadConfig() (cfg Config, err error) {
	if err = confire.Process(Prefix, &cfg); err != nil {
		return cfg, err
	}

	// The Conn cannot be set from the environment, so make sure it is nil!
	cfg.Conn = nil
	return cfg, nil
}

func (c Config) Validate() (err error) {
	if c.DatabaseURL == "" && !c.ManagedDB {
		err = confire.Join(err, confire.Invalid("radish", "database_url", "either the database DSN or a connection must be provided (specify RADISH_MANAGED_DB=1)"))
	}

	if c.NumWorkers < 1 {
		err = confire.Join(err, confire.Invalid("radish", "num_workers", "the number of workers must be at least 1"))
	}

	if c.TaskTimeout <= 0 {
		err = confire.Join(err, confire.Required("radish", "timeout"))
	}

	if c.PollInterval <= 0 {
		err = confire.Join(err, confire.Required("radish", "poll_interval"))
	}

	return err
}
