//go:build !windows

package app

// StartTray only has a Windows notification-area icon so far. On macOS and
// Linux the Start menu/Launchpad entries and the settings page's Quit button
// cover the same actions.
func StartTray(open, quit func()) (stop func()) { return func() {} }
