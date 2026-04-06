package port

type SystemInfoReader interface {
	ReadSystemInfo() (string, error)
}
