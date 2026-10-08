package backend

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func windowsRegistryString(path, name string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, _ := key.GetStringValue(name)
	return strings.TrimSpace(value)
}

func platformHostDetails(details HostDetails) HostDetails {
	const bios = `HARDWARE\DESCRIPTION\System\BIOS`
	if model := windowsRegistryString(bios, "SystemProductName"); model != "" {
		manufacturer := windowsRegistryString(bios, "SystemManufacturer")
		if manufacturer != "" && !strings.Contains(strings.ToLower(model), strings.ToLower(manufacturer)) {
			model = manufacturer + " " + model
		}
		details.Model = model
	}
	if cpu := windowsRegistryString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString"); cpu != "" {
		details.Chip = cpu
	}
	var memoryKiB uint64
	getMemory := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPhysicallyInstalledSystemMemory")
	if err := getMemory.Find(); err == nil {
		if ok, _, _ := getMemory.Call(uintptr(unsafe.Pointer(&memoryKiB))); ok != 0 && memoryKiB > 0 {
			details.Memory = fmt.Sprintf("%g GB", float64(memoryKiB)/(1<<20))
		}
	}
	const currentVersion = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	version := windows.RtlGetVersion()
	product := windowsRegistryString(currentVersion, "ProductName")
	if product == "" {
		product = "Windows"
	}
	// Windows 11 retains Windows 10's product string on some installations.
	const workstation = 1 // OSVERSIONINFOEX.wProductType: VER_NT_WORKSTATION.
	if version.BuildNumber >= 22000 && version.ProductType == workstation {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	if release := windowsRegistryString(currentVersion, "DisplayVersion"); release != "" {
		product += " " + release
	}
	details.System = fmt.Sprintf("%s (build %d)", product, version.BuildNumber)
	return details
}
