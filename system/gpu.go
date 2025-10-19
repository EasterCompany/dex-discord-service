package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Utilization float64
	MemoryUsed  float64
	MemoryTotal float64
}

func GetGPUInfo() (*GPUInfo, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi command failed: %w", err)
	}

	// Trim any extra whitespace and split by comma
	fields := strings.Split(strings.TrimSpace(string(output)), ", ")
	if len(fields) != 3 {
		return nil, fmt.Errorf("unexpected output format from nvidia-smi: got %d fields, expected 3", len(fields))
	}

	utilization, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPU utilization: %w", err)
	}

	memoryUsed, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse used GPU memory: %w", err)
	}

	memoryTotal, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total GPU memory: %w", err)
	}

	return &GPUInfo{
		Utilization: utilization,
		MemoryUsed:  memoryUsed,
		MemoryTotal: memoryTotal,
	}, nil
}

func IsNvidiaGPUInstalled() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}
