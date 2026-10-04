package osboundary

import (
	"log"
	"log/slog"
	"os/exec"
)

func Production() {
	_ = exec.Command("unused")
	log.Print("forbidden") // want "use of `log.Print` forbidden.*injected logger"
	_ = slog.Default()     // want "use of `slog.Default` forbidden.*injected logger"
}
