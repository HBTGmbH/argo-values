package logger

import (
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Log is the global logger instance
var Log = logrus.New()

func init() {
	Init("debug", "json", "stdout")
}

// Init initializes the global logger
func Init(level, format, out string) {
	// Set output
	if strings.ToLower(out) == "stdout" {
		Log.Out = os.Stdout
	} else {
		Log.Out = os.Stderr
	}

	// Set the log level
	Log.SetLevel(getLogLevel(level))

	// Set formatter
	if strings.ToLower(format) == "json" {
		Log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}
}

// getLogLevel gets actual log level
func getLogLevel(level string) logrus.Level {
	switch strings.ToLower(level) {
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	case "fatal":
		return logrus.FatalLevel
	case "panic":
		return logrus.PanicLevel
	default:
		return logrus.InfoLevel
	}
}

// SetLevel sets the log level
func SetLevel(level logrus.Level) {
	Log.SetLevel(level)
}

// SetOutput sets the output destination
func SetOutput(w io.Writer) {
	Log.Out = w
}

// Debug logs at debug level
func Debug(args ...interface{}) {
	Log.Debug(args...)
}

// Info logs at info level
func Info(args ...interface{}) {
	Log.Info(args...)
}

// Warn logs at the warning level
func Warn(args ...interface{}) {
	Log.Warn(args...)
}

// Error logs at error level
func Error(args ...interface{}) {
	Log.Error(args...)
}

// Fatal logs at fatal level and exits
func Fatal(args ...interface{}) {
	Log.Fatal(args...)
}

// Debugf logs at debug level with formatting
func Debugf(format string, args ...interface{}) {
	Log.Debugf(format, args...)
}

// Infof logs at info level with formatting
func Infof(format string, args ...interface{}) {
	Log.Infof(format, args...)
}

// Warnf logs at the warning level with formatting
func Warnf(format string, args ...interface{}) {
	Log.Warnf(format, args...)
}

// Errorf logs at error level with formatting
func Errorf(format string, args ...interface{}) {
	Log.Errorf(format, args...)
}

// Fatalf logs at the fatal level with formatting and exits
func Fatalf(format string, args ...interface{}) {
	Log.Fatalf(format, args...)
}

// Panic logs at panic level
func Panic(args ...interface{}) {
	Log.Panic(args...)
}

// Panicf logs at panic level with formatting
func Panicf(format string, args ...interface{}) {
	Log.Panicf(format, args...)
}
