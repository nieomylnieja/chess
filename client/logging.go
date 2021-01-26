package main

import (
	"fmt"

	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func init() {
	config := zap.NewProductionConfig()
	config.Level.SetLevel(zap.DebugLevel)
	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize zap logger {err=%s}", err.Error()))
	}
	log = logger.Sugar()
}
