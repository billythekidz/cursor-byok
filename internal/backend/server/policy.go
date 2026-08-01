package server

type ExecutionMode string

const (
	// ModeLocal indicates local mode, used when the request is handled directly.
	ModeLocal ExecutionMode = "local"
	// ModeUpstream indicates direct-upstream mode, used when requests are forwarded to the original address.
	ModeUpstream ExecutionMode = "upstream"
)

func parseExecutionMode(value string) ExecutionMode {
	switch value {
	case string(ModeUpstream):
		return ModeUpstream
	default:
		return ModeLocal
	}
}
