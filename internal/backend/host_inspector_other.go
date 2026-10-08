//go:build !darwin && !windows

package backend

func platformHostDetails(details HostDetails) HostDetails { return details }
