package app

import (
	"context"
	"fmt"
	stdnet "net"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type SystemMetrics struct {
	CollectedAt time.Time      `json:"collectedAt"`
	CPU         *CPUMetrics    `json:"cpu"`
	Memory      *MemoryMetrics `json:"memory"`
	Disk        *DiskMetrics   `json:"disk"`
	Network     *NetMetrics    `json:"network"`
	Warnings    []string       `json:"warnings,omitempty"`
}
type CPUMetrics struct {
	Cores        int     `json:"cores"`
	UsagePercent float64 `json:"usagePercent"`
}
type MemoryMetrics struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Available    uint64  `json:"available"`
	UsagePercent float64 `json:"usagePercent"`
}
type DiskMetrics struct {
	Path         string  `json:"path"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Available    uint64  `json:"available"`
	UsagePercent float64 `json:"usagePercent"`
}
type NetMetrics struct {
	ReceiveRate *float64 `json:"receiveRate"`
	SendRate    *float64 `json:"sendRate"`
}
type networkCounter struct{ received, sent uint64 }
type resourceSampler struct {
	sync.Mutex
	cached    *SystemMetrics
	network   map[string]networkCounter
	networkAt time.Time
}

func networkRates(previous, current map[string]networkCounter, elapsed time.Duration) (float64, float64) {
	if elapsed <= 0 {
		return 0, 0
	}
	var received, sent uint64
	for name, now := range current {
		before, exists := previous[name]
		if !exists {
			continue
		}
		if now.received >= before.received {
			received += now.received - before.received
		}
		if now.sent >= before.sent {
			sent += now.sent - before.sent
		}
	}
	return float64(received) / elapsed.Seconds(), float64(sent) / elapsed.Seconds()
}

func readNetwork(ctx context.Context) (map[string]networkCounter, error) {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, nic := range interfaces {
		if nic.Flags&stdnet.FlagLoopback == 0 && nic.Flags&stdnet.FlagUp != 0 {
			active[nic.Name] = true
		}
	}
	counters, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	result := map[string]networkCounter{}
	for _, nic := range counters {
		if active[nic.Name] {
			result[nic.Name] = networkCounter{nic.BytesRecv, nic.BytesSent}
		}
	}
	return result, nil
}

// 按需采样、两秒缓存：多个网页共用同一基线，不阻塞下载队列的状态锁。
func (s *resourceSampler) sample(ctx context.Context, downloadDir string) SystemMetrics {
	s.Lock()
	defer s.Unlock()
	if s.cached != nil && time.Since(s.cached.CollectedAt) < 2*time.Second {
		return *s.cached
	}
	result := SystemMetrics{}
	if usage, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(usage) > 0 {
		result.CPU = &CPUMetrics{Cores: runtime.NumCPU(), UsagePercent: usage[0]}
	} else {
		result.Warnings = append(result.Warnings, "CPU 使用率暂时不可用")
	}
	if usage, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		result.Memory = &MemoryMetrics{usage.Total, usage.Used, usage.Available, usage.UsedPercent}
	} else {
		result.Warnings = append(result.Warnings, "内存使用情况暂时不可用")
	}
	if usage, err := disk.UsageWithContext(ctx, downloadDir); err == nil {
		result.Disk = &DiskMetrics{downloadDir, usage.Total, usage.Used, usage.Free, usage.UsedPercent}
	} else {
		result.Warnings = append(result.Warnings, fmt.Sprintf("下载目录所在磁盘暂时不可用：%v", err))
	}
	if counters, err := readNetwork(ctx); err == nil {
		now := time.Now()
		result.Network = &NetMetrics{}
		if s.network != nil {
			rx, tx := networkRates(s.network, counters, now.Sub(s.networkAt))
			result.Network.ReceiveRate, result.Network.SendRate = &rx, &tx
		}
		s.network, s.networkAt = counters, now
	} else {
		s.network = nil
		result.Warnings = append(result.Warnings, "网络速率暂时不可用")
	}
	result.CollectedAt = time.Now()
	s.cached = &result
	return result
}
