package main

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func init() {
	var cfg struct {
		Debug bool `default:"false"`
	}
	envconfig.MustProcess("", &cfg)
	config := zap.NewProductionConfig()
	if cfg.Debug {
		config.Level.SetLevel(zap.DebugLevel)
	}
	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize zap logger {err=%s}", err.Error()))
	}
	log = logger.Sugar()
}
