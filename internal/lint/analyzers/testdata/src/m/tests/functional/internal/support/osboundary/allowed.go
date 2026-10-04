package osboundary

import "os/exec"

func Allowed() { _ = exec.Command("unused") }
