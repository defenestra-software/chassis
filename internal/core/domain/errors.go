package domain

import "errors"

var (
	ErrDeviceNotFound   = errors.New("device not found")
	ErrPermissionDenied = errors.New("permission denied by policy")
	ErrDeviceBusy       = errors.New("device is busy")
	ErrDriverNotFound   = errors.New("driver not found")
	ErrInvalidPath      = errors.New("invalid device path")
	ErrModuleLoadFailed = errors.New("failed to load kernel module")
	ErrDriverNotBound   = errors.New("no driver is currently bound to this device")
)

type OperationError struct {
	Op       string
	DeviceID string
	Err      error
}

func (e *OperationError) Error() string {
	return e.Op + " " + e.DeviceID + ": " + e.Err.Error()
}

func (e *OperationError) Unwrap() error {
	return e.Err
}
