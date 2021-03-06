// +build darwin

package crash

import (
	"log"
	"os"
	"syscall"
)

// NewCrashLog set crash log
func NewCrashLog(file string) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("open crash log file error. %v", err)
	} else {
		_ = syscall.Dup2(int(f.Fd()), 2)
	}
}
