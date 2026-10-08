package backend

import (
	"os"
	"os/user"
	"runtime"
)

type HostDetails struct{ Model, Chip, Memory, System, User string }
type HostInspector struct{}

func (HostInspector) Details() HostDetails {
	details := HostDetails{Model: "Unavailable", Chip: "Unavailable", Memory: "Unavailable", System: runtime.GOOS, User: os.Getenv("USER")}
	if u, err := user.Current(); err == nil {
		details.User = u.Username
	}
	return platformHostDetails(details)
}
