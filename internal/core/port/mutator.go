package port

type DriverMutator interface {
	Unbind(deviceID string, busType string) error
	Bind(deviceID string, driverName string) error

	DriverProbe(deviceID string) error

	LoadModule(moduleName string) error
	UnloadModule(moduleName string) error

	Close() error
}
