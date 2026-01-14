package db_session

import (
	"sync"

	"gorm.io/gorm/logger"
)

const (
	disable = "disable"
)

var once sync.Once

// LoggerReconfigurable allows runtime reconfiguration of the database logger
type LoggerReconfigurable interface {
	ReconfigureLogger(level logger.LogLevel)
}
