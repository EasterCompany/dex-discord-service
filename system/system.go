package system

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type SysInfo struct {
	CPUModel       string
	CPUCoreCount   int32
	CPUThreadCount int
	CPUSpeed       float64
	TotalMemory    uint64
}

func GetSysInfo() (*SysInfo, error) {
	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	if len(cpuInfo) == 0 {
		return nil, fmt.Errorf("no CPU info found")
	}

	coreCount, err := cpu.Counts(false)
	if err != nil {
		return nil, err
	}

	threadCount, err := cpu.Counts(true)
	if err != nil {
		return nil, err
	}

	virtualMem, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	return &SysInfo{
		CPUModel:       cpuInfo[0].ModelName,
		CPUCoreCount:   int32(coreCount),
		CPUThreadCount: threadCount,
		CPUSpeed:       cpuInfo[0].Mhz,
		TotalMemory:    virtualMem.Total,
	}, nil
}

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
