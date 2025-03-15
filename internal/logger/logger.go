package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
	ErrorLogger *log.Logger
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
)

func init() {
	InfoLogger = log.New(os.Stdout, fmt.Sprintf("%s[INFO]%s ", colorGreen, colorReset), log.Ldate|log.Ltime)
	WarnLogger = log.New(os.Stdout, fmt.Sprintf("%s[WARN]%s ", colorYellow, colorReset), log.Ldate|log.Ltime)
	ErrorLogger = log.New(os.Stdout, fmt.Sprintf("%s[ERROR]%s ", colorRed, colorReset), log.Ldate|log.Ltime)
}

// Info logs information messages
func Info(format string, v ...interface{}) {
	InfoLogger.Printf(format, v...)
}

// Warn logs warning messages
func Warn(format string, v ...interface{}) {
	WarnLogger.Printf(format, v...)
}

// Error logs error messages
func Error(format string, v ...interface{}) {
	ErrorLogger.Printf(format, v...)
}

// Fatal logs error message and exits
func Fatal(format string, v ...interface{}) {
	ErrorLogger.Printf(format, v...)
	os.Exit(1)
}

// RequestLog logs HTTP request information
func RequestLog(method, path, ip string, duration time.Duration) {
	InfoLogger.Printf("%s[%s] %s from %s took %v", colorBlue, method, path, ip, duration)
}
