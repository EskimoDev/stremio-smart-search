//go:build !windows

package app

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
