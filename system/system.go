// Package system provides functions for retrieving system metrics.
package system

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type StorageInfo struct {
	Device      string
	MountPoint  string
	Total       uint64
	Used        uint64
	UsedPercent float64
}

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name        string        `json:"name"`
	MountPoints []string      `json:"mountpoints"`
	Size        uint64        `json:"size"`
	Type        string        `json:"type"`
	Children    []lsblkDevice `json:"children"`
}

func GetStorageInfo() ([]StorageInfo, error) {
	cmd := exec.Command("lsblk", "-Jb", "-o", "NAME,MOUNTPOINTS,SIZE,TYPE")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	var lsblkOut lsblkOutput
	if err := json.Unmarshal(output, &lsblkOut); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lsblk output: %w", err)
	}

	var storageDevices []StorageInfo
	deviceProcessed := make(map[string]bool)

	for _, device := range lsblkOut.BlockDevices {
		if device.Type == "disk" {
			for _, child := range device.Children {
				if deviceProcessed[child.Name] {
					continue
				}
				if len(child.MountPoints) > 0 {
					mountPoint := getBestMountPoint(child.MountPoints)
					if mountPoint != "" {
						storageDevices = append(storageDevices, processDevice(child, mountPoint))
						deviceProcessed[child.Name] = true
					}
				}
			}
		}
	}

	return storageDevices, nil
}

func getBestMountPoint(mounts []string) string {
	if len(mounts) == 0 {
		return ""
	}

	// Prefer the root mount point
	for _, m := range mounts {
		if m == "/" {
			return "/"
		}
	}

	// Otherwise, return the shortest mount point path
	sort.Strings(mounts)
	return mounts[0]
}

func processDevice(device lsblkDevice, mountPoint string) StorageInfo {
	info := StorageInfo{
		Device:     device.Name,
		MountPoint: mountPoint,
		Total:      device.Size,
	}

	if info.MountPoint != "" {
		usage, err := disk.Usage(info.MountPoint)
		if err == nil {
			info.Used = usage.Used
			info.UsedPercent = usage.UsedPercent
		}
	}
	return info
}

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
