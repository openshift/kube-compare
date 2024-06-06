package logging

import (
	"k8s.io/klog/v2"
)

var logger *Logger

type Logger struct {
	Error  func(args ...any)
	Errorf func(format string, args ...any)
	Fatal  func(args ...any)
	Fatalf func(format string, args ...any)
	Exit   func(args ...any)
	Exitf  func(format string, args ...any)
}

func SetLogger(newLogger *Logger) {
	logger = newLogger
}

func DefaultLogger() *Logger {
	return &Logger{
		Error:  klog.Error,
		Errorf: klog.Errorf,
		Fatal:  klog.Fatal,
		Fatalf: klog.Fatalf,
		Exit:   klog.Exit,
		Exitf:  klog.Exitf,
	}
}

func GetLogger() *Logger {
	return logger
}

func init() {
	logger = DefaultLogger()
}
