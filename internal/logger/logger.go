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
	Init("debug", "json", "stderr")
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
//
//goland:noinspection GoUnusedExportedFunction
func SetLevel(level logrus.Level) {
	Log.SetLevel(level)
}

// SetOutput sets the output destination
//
//goland:noinspection GoUnusedExportedFunction
func SetOutput(w io.Writer) {
	Log.Out = w
}

// Debug logs at debug level
func Debug(args ...any) {
	Log.Debug(args...)
}

// Info logs at info level
func Info(args ...any) {
	Log.Info(args...)
}

// Warn logs at the warning level
//
//goland:noinspection GoUnusedExportedFunction
func Warn(args ...any) {
	Log.Warn(args...)
}

// Error logs at error level
func Error(args ...any) {
	Log.Error(args...)
}

// Fatal logs at fatal level and exits
func Fatal(args ...any) {
	Log.Fatal(args...)
}

// Debugf logs at debug level with formatting
func Debugf(format string, args ...any) {
	Log.Debugf(format, args...)
}

// Infof logs at info level with formatting
//
//goland:noinspection GoUnusedExportedFunction
func Infof(format string, args ...any) {
	Log.Infof(format, args...)
}

// Warnf logs at the warning level with formatting
func Warnf(format string, args ...any) {
	Log.Warnf(format, args...)
}

// Errorf logs at error level with formatting
func Errorf(format string, args ...any) {
	Log.Errorf(format, args...)
}

// Fatalf logs at the fatal level with formatting and exits
func Fatalf(format string, args ...any) {
	Log.Fatalf(format, args...)
}

// Panic logs at panic level
//
//goland:noinspection GoUnusedExportedFunction
func Panic(args ...any) {
	Log.Panic(args...)
}

// Panicf logs at panic level with formatting
//
//goland:noinspection GoUnusedExportedFunction
func Panicf(format string, args ...any) {
	Log.Panicf(format, args...)
}
