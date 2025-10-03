package maxproc

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Set adjusts GOMAXPROCS to match the CPU quota detected in the current cgroup.
// It returns a function that restores the previous value and an error if the
// detection logic failed in an unexpected way.
func Set() (func(), error) {
	previous := runtime.GOMAXPROCS(0)

	maxProcs, err := recommendedProcs()
	if err != nil {
		return func() { runtime.GOMAXPROCS(previous) }, err
	}

	if maxProcs > 0 && maxProcs != previous {
		runtime.GOMAXPROCS(maxProcs)
	}

	return func() { runtime.GOMAXPROCS(previous) }, nil
}

func recommendedProcs() (int, error) {
	if env := os.Getenv("GOMAXPROCS"); env != "" {
		if value, err := strconv.Atoi(env); err == nil && value > 0 {
			return value, nil
		}
	}

	if quota, period, ok := readCgroupV2(); ok {
		return quotaToProcs(quota, period), nil
	}

	if quota, period, ok := readCgroupV1(); ok {
		return quotaToProcs(quota, period), nil
	}

	return 0, nil
}

func quotaToProcs(quota, period int64) int {
	if quota <= 0 || period <= 0 {
		return 0
	}

	procs := int((quota + period - 1) / period)
	if procs < 1 {
		procs = 1
	}
	return procs
}

func readCgroupV2() (quota, period int64, ok bool) {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, 0, false
	}

	if fields[0] == "max" {
		return 0, 0, true
	}

	quota, err = strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	period, err = strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return quota, period, true
}

func readCgroupV1() (quota, period int64, ok bool) {
	quotaBytes, err := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if err != nil {
		return 0, 0, false
	}

	periodBytes, err := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if err != nil {
		return 0, 0, false
	}

	quota, err = strconv.ParseInt(strings.TrimSpace(string(quotaBytes)), 10, 64)
	if err != nil {
		return 0, 0, false
	}

	period, err = strconv.ParseInt(strings.TrimSpace(string(periodBytes)), 10, 64)
	if err != nil {
		return 0, 0, false
	}

	if quota == -1 {
		return 0, 0, true
	}

	return quota, period, true
}
