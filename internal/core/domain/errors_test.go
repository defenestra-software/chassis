package domain

import (
	"errors"
	"testing"
)

func TestOperationError(t *testing.T) {
	t.Run("Error message format", func(t *testing.T) {
		err := &OperationError{
			Op:       "unbind",
			DeviceID: "0000:01:00.0",
			Err:      ErrDeviceBusy,
		}

		want := "unbind 0000:01:00.0: device is busy"
		if got := err.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlying := ErrDeviceBusy
		err := &OperationError{
			Op:       "bind",
			DeviceID: "0000:01:00.0",
			Err:      underlying,
		}

		if !errors.Is(err, underlying) {
			t.Error("errors.Is() should match the underlying error")
		}
	})

	t.Run("errors.As extracts OperationError", func(t *testing.T) {
		err := &OperationError{
			Op:       "unbind",
			DeviceID: "0000:03:00.0",
			Err:      ErrDeviceNotFound,
		}

		var opErr *OperationError
		if !errors.As(err, &opErr) {
			t.Fatal("errors.As() should match OperationError")
		}
		if opErr.Op != "unbind" {
			t.Errorf("Op = %q, want %q", opErr.Op, "unbind")
		}
		if opErr.DeviceID != "0000:03:00.0" {
			t.Errorf("DeviceID = %q, want %q", opErr.DeviceID, "0000:03:00.0")
		}
	})
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrDeviceNotFound", ErrDeviceNotFound, "device not found"},
		{"ErrPermissionDenied", ErrPermissionDenied, "permission denied by policy"},
		{"ErrDeviceBusy", ErrDeviceBusy, "device is busy"},
		{"ErrDriverNotFound", ErrDriverNotFound, "driver not found"},
		{"ErrInvalidPath", ErrInvalidPath, "invalid device path"},
		{"ErrModuleLoadFailed", ErrModuleLoadFailed, "failed to load kernel module"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
