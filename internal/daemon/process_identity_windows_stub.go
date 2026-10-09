//go:build !windows

package daemon

func processIdentityWindows(uint32) ProcessIdentity { return ProcessIdentity{} }
