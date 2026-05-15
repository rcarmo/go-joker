package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/pkg/profile"
	. "github.com/rcarmo/go-joker/core"
)

var runningProfile interface {
	Stop()
}

var runningCPUProfileFile *os.File

func startProfiling() error {
	if cpuProfileName != "" {
		switch profilerType {
		case "pkg/profile":
			runningProfile = profile.Start(profile.ProfilePath(cpuProfileName))
		case "runtime/pprof":
			name := cpuProfileName
			f, err := os.Create(name)
			if err != nil {
				cpuProfileName = ""
				return fmt.Errorf("could not create CPU profile `%s`: %w", name, err)
			}
			if cpuProfileRateFlag {
				runtime.SetCPUProfileRate(cpuProfileRate)
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				f.Close()
				cpuProfileName = ""
				return fmt.Errorf("could not start CPU profile `%s`: %w", name, err)
			}
			runningCPUProfileFile = f
			fmt.Fprintf(Stderr, "Profiling started at rate=%d. See file `%s'.\n",
				cpuProfileRate, cpuProfileName)
		default:
			return fmt.Errorf("unrecognized profiler: %s (use 'pkg/profile' or 'runtime/pprof')", profilerType)
		}
	}
	return nil
}

func finish() {
	if runningProfile != nil {
		runningProfile.Stop()
		runningProfile = nil
	} else if cpuProfileName != "" {
		pprof.StopCPUProfile()
		if runningCPUProfileFile != nil {
			if err := runningCPUProfileFile.Close(); err != nil {
				fmt.Fprintf(Stderr, "Error: Could not close CPU profile `%s': %v\n", cpuProfileName, err)
			}
			runningCPUProfileFile = nil
		}
		fmt.Fprintf(Stderr, "Profiling stopped. See file `%s'.\n", cpuProfileName)
		cpuProfileName = ""
	}

	if memProfileName != "" {
		f, err := os.Create(memProfileName)
		if err != nil {
			fmt.Fprintf(Stderr, "Error: Could not create memory profile `%s': %v\n",
				memProfileName, err)
			memProfileName = ""
			return
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(Stderr, "Error: Could not write memory profile `%s': %v\n",
				memProfileName, err)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(Stderr, "Error: Could not close memory profile `%s': %v\n",
				memProfileName, err)
		}
		fmt.Fprintf(Stderr, "Memory profile rate=%d written to `%s'.\n",
			runtime.MemProfileRate, memProfileName)
		memProfileName = ""
	}
}
