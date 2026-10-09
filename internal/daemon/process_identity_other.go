//go:build !darwin

package daemon

func processIdentityDarwin(uint32) ProcessIdentity { return ProcessIdentity{} }
