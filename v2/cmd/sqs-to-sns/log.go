package main

import (
	"fmt"
	"log/slog"
	"os"
)

func infof(format string, v ...any) {
	slog.Info(fmt.Sprintf(format, v...))
}

func fatalf(format string, v ...any) {
	slog.Error("FATAL: " + fmt.Sprintf(format, v...))
	os.Exit(1)
}
