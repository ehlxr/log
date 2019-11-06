package log

import (
	"testing"
)

func TestLog(t *testing.T) {
	Debugf("this is %s message", "debug")
	Infof("this is %s message", "info")
	Errorf("this is %s message", "error")
	// Panicf("this is %s message", "panic")
}


func TestLogWithConfig(t *testing.T) {
	lc := NewLogConfig()
	_ = lc.Level.Set("info")
	lc.Name="main"
	lc.Init()

	Debugf("this is %s message", "debug")
	Infof("this is %s message", "info")
	Errorf("this is %s message", "error")
	// Panicf("this is %s message", "panic")
}
