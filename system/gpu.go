package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Name        string
	Utilization float64
	MemoryUsed  float64
	MemoryTotal float64
}

func GetGPUInfo() ([]GPUInfo, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi command failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var gpus []GPUInfo

	for _, line := range lines {
		fields := strings.Split(line, ", ")
		if len(fields) != 4 {
			return nil, fmt.Errorf("unexpected output format from nvidia-smi: got %d fields, expected 4", len(fields))
		}

		utilization, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GPU utilization: %w", err)
		}

		memoryUsed, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse used GPU memory: %w", err)
		}

		memoryTotal, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total GPU memory: %w", err)
		}

		gpus = append(gpus, GPUInfo{
			Name:        fields[0],
			Utilization: utilization,
			MemoryUsed:  memoryUsed,
			MemoryTotal: memoryTotal,
		})
	}

	return gpus, nil
}

func IsNvidiaGPUInstalled() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}
