package logging

import (
	"go.uber.org/zap"
	"log"
)

var (
	logger *zap.Logger
	Logger *zap.SugaredLogger
)

func Initialize(debugMode bool) {
	var err error
	if debugMode {
		logger, err = zap.NewDevelopment()

	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatalf("Error configuring logger. %v", err)
	}
	Logger = logger.Sugar()
}

func Release() {
	if logger != nil {
		_ = logger.Sync()
	}
}
