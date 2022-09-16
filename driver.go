package dbtest

import (
	"time"
)

type Driver interface {
	SetConfig(config *Config)
	Stop()
	Client() interface{}
	Wipe()
}

var drivers = make(map[string]func() Driver)

func New(name string) Driver {
	return NewWithConfig(name, &Config{
		Timeout: time.Second * 30,
	})
}

func NewWithConfig(name string, config *Config) Driver {
	var driver Driver

	v, ok := drivers[name]
	if !ok {
		panic("dbtest: driver not found")
	}

	driver = v()
	driver.SetConfig(config)

	return driver
}

func RegisterDriver(name string, driver func() Driver) {
	drivers[name] = driver
}
