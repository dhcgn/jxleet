package cjxl

// ProcessResources is a point-in-time snapshot of one running cjxl child:
// cumulative CPU time (user + kernel) and current working-set RAM.
// Sampled on a fixed cadence while the queue row shows processing; before the
// PID exists or after the process exits the row keeps its placeholder.
// ref:jl:tech.tool.resources
type ProcessResources struct {
	PID            int     `json:"pid"`
	CPUTimeSeconds float64 `json:"cpuTimeSeconds"`
	MemoryBytes    uint64  `json:"memoryBytes"`
}
