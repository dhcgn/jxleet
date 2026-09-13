//go:build !windows

package cjxl

import "errors"

// GetProcessResources is unavailable outside Windows: jxleet is a
// Windows-only app and per-process sampling stays Windows-only.
// ref:jl:tech.tool.resources
func GetProcessResources(pid int) (ProcessResources, error) {
	return ProcessResources{}, errors.New("cjxl: process resources are only available on Windows")
}
