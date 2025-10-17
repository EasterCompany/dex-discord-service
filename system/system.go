// eastercompany/dex-discord-interface/system/system.go
package system

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// GetCPUUsage returns the current CPU usage as a percentage
func GetCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(0, false)
	if err != nil {
		return 0, err
	}
	if len(percentages) == 0 {
		return 0, fmt.Errorf("could not get CPU usage")
	}
	return percentages[0], nil
}

// GetMemoryUsage returns the current memory usage as a percentage
func GetMemoryUsage() (float64, error) {
	virtualMem, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return virtualMem.UsedPercent, nil
}
