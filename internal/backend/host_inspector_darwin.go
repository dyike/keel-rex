package backend

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func platformHostDetails(details HostDetails) HostDetails {
	read := func(name string) string {
		b, _ := exec.Command("sysctl", "-n", name).Output()
		return strings.TrimSpace(string(b))
	}
	chip, hw := read("machdep.cpu.brand_string"), read("hw.model")
	memory, _ := strconv.ParseUint(read("hw.memsize"), 10, 64)
	version, _ := exec.Command("sw_vers", "-productVersion").Output()
	model := hw
	for _, family := range []string{"Macmini", "MacBookPro", "MacBookAir", "MacStudio", "iMac", "MacPro"} {
		if strings.HasPrefix(hw, family) {
			model = map[string]string{"Macmini": "Mac mini", "MacBookPro": "MacBook Pro", "MacBookAir": "MacBook Air", "MacStudio": "Mac Studio", "MacPro": "Mac Pro", "iMac": "iMac"}[family]
			break
		}
	}
	details.Model, details.Chip = model, chip
	if memory > 0 {
		details.Memory = fmt.Sprintf("%.0f GB", float64(memory)/(1<<30))
	}
	details.System = "macOS " + strings.TrimSpace(string(version))
	return details
}
