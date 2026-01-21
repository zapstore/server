package sqlite

import (
	"errors"
	"fmt"
)

type Config struct {
	Path string `env:"SQLITE_PATH"`
}

func NewConfig() Config {
	return Config{
		Path: "store.db",
	}
}

func (c Config) Validate() error {
	if c.Path == "" {
		return errors.New("path is not set")
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf("Sqlite:\n"+
		"\tPath: %s\n",
		c.Path)
}
