//go:build !darwin && !windows

package daemon

func processIdentityDarwin(uint32) ProcessIdentity { return ProcessIdentity{} }
