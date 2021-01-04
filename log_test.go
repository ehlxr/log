package log

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLog(t *testing.T) {
	Debugf("this is %s message", "debug")
	Infof("this is %s message", "info")
	Errorf("this is %s message", "error")
	// Panicf("this is %s message", "panic")
}

func TestLogWithConfig(t *testing.T) {
	config := NewLogConfig()
	_ = config.Level.Set("debug")
	config.Name = "main"
	// config.Fields = []zap.Field{zap.String("traceid", "12123123123")}

	config.Init()

	Fields("traceid", float64(21221212122))
	Debugf("this is %s message", "debug")
	config.Init()
	Fields(zap.String("traceid", "12123123123"))
	Infof("this is %s message", "info")
	// Errorf("this is %s message", "error")
	// Panicf("this is %s message", "panic")
}

func TestLogWithNew(t *testing.T) {
	config := NewLogConfig()
	_ = config.Level.Set("debug")
	config.Name = "main"

	logger := config.New()

	log := With(logger, "traceid", float64(21221212122), "request", "[POST]/hello/v2")
	log.Debugf("this is %s message", "debug")
	log.Infof("this is %s message", "info")
}

func TestLogRote(t *testing.T) {
	lc := NewLogConfig()
	lc.MaxSize = 1

	lc.Init()

	for {
		Infof("this is %s message", "info")
		time.Sleep(time.Millisecond * 1)
	}
}
