package main

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
	log "github.com/sirupsen/logrus"
)

const (
	CpuObserveInterval = 1000 * time.Millisecond
)

type HostInfo struct {
	// Misc static info
	Hostname string
	Tags     map[string]string

	// Static resource info
	NumCores int    // Number
	TotalMem uint64 // Bytes

	// Resource usage info
	UsedCpuCores []float64 // Percent, per core
	UsedCpu      float64   // Percent, avg of all cores
	UsedMem      uint64    // Bytes
	NumProcs     int       // Number
	Goroutines   int       // Number
}

func (engine *SubprocessEngine) getInfo() *HostInfo {
	coreUsage := engine.currentCpuUsage
	avgCoreUsage := float64(0)
	for _, usage := range coreUsage {
		avgCoreUsage += usage
	}
	avgCoreUsage /= float64(len(coreUsage))

	virtMem, err := mem.VirtualMemory()
	if err != nil {
		log.Warnln("Failed to obtain virtual memory info:", err)
	}

	pids, err := process.Pids()
	if err != nil {
		log.Warnln("Failed to obtain number of running processes:", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Warnln("Failed to obtain the hostname:", err)
	}

	return &HostInfo{
		Tags:         engine.Tags,
		Hostname:     hostname,
		NumCores:     runtime.NumCPU(),
		TotalMem:     virtMem.Total,
		NumProcs:     len(pids),
		Goroutines:   runtime.NumGoroutine(),
		UsedCpuCores: coreUsage,
		UsedCpu:      avgCoreUsage,
		UsedMem:      virtMem.Used,
	}
}

func (engine *SubprocessEngine) ObserveCpuTimes() {
	for {
		times, err := cpu.Times(true)
		if err != nil {
			log.Warnln("Failed to read CPU usage:", err)
		}

		lastTimes := engine.lastCpuTimes
		engine.lastCpuTimes = times
		if lastTimes == nil {
			continue
		}
		if len(times) != len(lastTimes) {
			log.Warnln("The number of reported CPU times changed from", len(lastTimes), "to", len(times))
			continue
		}

		usage := make([]float64, len(times))
		for i, oldTime := range lastTimes {
			newTime := times[i]
			percent := getCpuPercent(oldTime, newTime)
			usage[i] = percent
		}

		// Replace the slice pointer in the end to avoid race conditions by writing the entries.
		engine.currentCpuUsage = usage

		time.Sleep(CpuObserveInterval)
	}
}

func getCpuPercent(oldTime cpu.TimesStat, newTime cpu.TimesStat) float64 {
	// Calculation based on https://github.com/shirou/gopsutil/blob/master/cpu/cpu_unix.go
	t1All, t1Busy := getCpuTimes(oldTime)
	t2All, t2Busy := getCpuTimes(newTime)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 1
	}
	return (t2Busy - t1Busy) / (t2All - t1All) * 100
}

func getCpuTimes(t cpu.TimesStat) (float64, float64) {
	busy := t.User + t.System + t.Nice + t.Irq +
		t.Softirq + t.Steal + t.Guest + t.GuestNice + t.Stolen
	return busy + t.Idle + t.Iowait, busy
}
