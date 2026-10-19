package app

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNetworkRatesIgnoreNewRemovedAndResetInterfaces(t *testing.T) {
	previous := map[string]networkCounter{"en0": {100, 200}, "removed": {999999, 999999}}
	current := map[string]networkCounter{"en0": {500, 1000}, "new": {999999, 999999}}
	rx, tx := networkRates(previous, current, 2*time.Second)
	if rx != 200 || tx != 400 {
		t.Fatalf("速率应按同一网卡的字节增量计算：%v %v", rx, tx)
	}
	rx, tx = networkRates(previous, map[string]networkCounter{"en0": {1, 2}}, time.Second)
	if rx != 0 || tx != 0 {
		t.Fatal("计数器重置不能产生负数或无符号溢出")
	}
	rx, tx = networkRates(previous, current, 0)
	if rx != 0 || tx != 0 {
		t.Fatal("时间未推进时不能除以零")
	}
}

func TestMetricsPartialFailureDoesNotInventDiskValues(t *testing.T) {
	a := testApp(t)
	if err := os.Remove(a.downloadDir); err != nil {
		t.Fatal(err)
	}
	result := a.metrics.sample(context.Background(), a.downloadDir)
	if result.Disk != nil || len(result.Warnings) == 0 {
		t.Fatal("磁盘采集失败应明确不可用，不能伪造 0%")
	}
	if result.Memory == nil || result.CPU == nil {
		t.Fatal("磁盘失败不能让其他资源一起消失")
	}
}
