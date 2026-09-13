package cjxl

// ProcessResources is a point-in-time snapshot of one running cjxl child:
// CPU usage in percent of total machine capacity (0-100) and current
// working-set RAM. Sampled on a fixed cadence while the queue row shows
// processing; before the PID exists or after the process exits the row keeps
// its placeholder.
// ref:jl:tech.tool.resources
type ProcessResources struct {
	PID         int     `json:"pid"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
}
