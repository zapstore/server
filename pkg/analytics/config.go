package analytics

import (
	"errors"
	"fmt"
	"time"
)

type Config struct {
	// FlushInterval is the interval at which the analytics engine flushes data to the database. Default is 1 minute.
	FlushInterval time.Duration `env:"ANALYTICS_FLUSH_INTERVAL"`

	// MaxBatchSize is the maximum number of events that can be batched before flushing to the database. Default is 100.
	MaxBatchSize int `env:"ANALYTICS_MAX_BATCH_SIZE"`
}

func NewConfig() Config {
	return Config{
		FlushInterval: time.Minute,
		MaxBatchSize:  100,
	}
}

func (c Config) Validate() error {
	if c.FlushInterval < time.Second {
		return errors.New("flush interval must be greater than 1s to avoid too many database writes")
	}
	if c.FlushInterval > time.Hour {
		return errors.New("flush interval must be less than 1h to avoid data loss in case of a server crash or restart")
	}
	if c.MaxBatchSize <= 0 {
		return errors.New("max batch size must be greater than 0")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Analytics:\n"+
		"\tFlush Interval: %s\n"+
		"\tMax Batch Size: %d\n",
		c.FlushInterval,
		c.MaxBatchSize)
}
