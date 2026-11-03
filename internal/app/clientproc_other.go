//go:build !windows && !linux && !darwin

package app

// ClientRunning cannot see other processes here, so standby relies on
// incoming searches alone.
func (a *App) ClientRunning() bool { return false }
