package scan

type State uint8

const (
	Pending State = iota
	Complete
	Partial
	Unknown
	Skipped
)

func (s State) String() string {
	switch s {
	case Complete:
		return "Complete"
	case Partial:
		return "Partial"
	case Unknown:
		return "Unknown"
	case Skipped:
		return "Skipped"
	default:
		return "Pending"
	}
}

type Entry struct {
	Path, Name       string
	IsDir, IsSymlink bool
}

type Progress struct {
	Current          string
	Completed, Total int
}
type Measurement struct {
	Bytes uint64
	State State
	Err   error
}
type Snapshot struct {
	Listing []Entry // Immediate entries captured by this scan, in display order.
	Entries map[string]Measurement
	State   State
	Err     error
}
