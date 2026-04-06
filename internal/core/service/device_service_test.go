package service

import (
	"errors"
	"log/slog"
	"testing"

	"defenestra.io/chassis/internal/core/domain"
)

type mockScanner struct {
	tree *domain.DeviceNode
	err  error
}

func (m *mockScanner) Scan() (*domain.DeviceNode, error) {
	return m.tree, m.err
}

func (m *mockScanner) GetDevice(id string) (*domain.DeviceNode, error) {
	return nil, nil
}

func (m *mockScanner) GetDevicesByType(dt domain.DeviceType) ([]*domain.DeviceNode, error) {
	return nil, nil
}

func (m *mockScanner) GetIOMMUGroup(groupID int) ([]*domain.DeviceNode, error) {
	return nil, nil
}

type mockMutator struct {
	unbindCalls []mockUnbindCall
	bindCalls   []mockBindCall
	probeCalls  []string
	unbindErr   error
	bindErr     error
}

type mockUnbindCall struct {
	DeviceID string
	BusType  string
}

type mockBindCall struct {
	DeviceID   string
	DriverName string
}

func (m *mockMutator) Unbind(deviceID, busType string) error {
	m.unbindCalls = append(m.unbindCalls, mockUnbindCall{deviceID, busType})
	return m.unbindErr
}

func (m *mockMutator) Bind(deviceID, driverName string) error {
	m.bindCalls = append(m.bindCalls, mockBindCall{deviceID, driverName})
	return m.bindErr
}

func (m *mockMutator) LoadModule(name string) error   { return nil }
func (m *mockMutator) UnloadModule(name string) error { return nil }
func (m *mockMutator) Close() error                   { return nil }

func (m *mockMutator) DriverProbe(deviceID string) error {
	m.probeCalls = append(m.probeCalls, deviceID)
	return nil
}

type mockListener struct{}

func (m *mockListener) Subscribe() (<-chan domain.HardwareEvent, error) {
	return make(chan domain.HardwareEvent), nil
}

func (m *mockListener) Stop() error { return nil }

func newTestService(mutator *mockMutator) *DeviceService {
	logger := slog.Default()
	scanner := &mockScanner{
		tree: &domain.DeviceNode{ID: "root", Name: "System"},
	}
	return NewDeviceService(scanner, mutator, &mockListener{}, logger)
}

func TestUnbindDriver(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mut := &mockMutator{}
		svc := newTestService(mut)

		err := svc.UnbindDriver("0000:01:00.0", "pci")
		if err != nil {
			t.Fatalf("UnbindDriver() error = %v", err)
		}

		if len(mut.unbindCalls) != 1 {
			t.Fatalf("expected 1 unbind call, got %d", len(mut.unbindCalls))
		}
		if mut.unbindCalls[0].DeviceID != "0000:01:00.0" {
			t.Errorf("DeviceID = %q, want %q", mut.unbindCalls[0].DeviceID, "0000:01:00.0")
		}
		if mut.unbindCalls[0].BusType != "pci" {
			t.Errorf("BusType = %q, want %q", mut.unbindCalls[0].BusType, "pci")
		}
	})

	t.Run("error wraps as OperationError", func(t *testing.T) {
		mut := &mockMutator{unbindErr: domain.ErrDeviceBusy}
		svc := newTestService(mut)

		err := svc.UnbindDriver("0000:01:00.0", "pci")
		if err == nil {
			t.Fatal("expected error")
		}

		var opErr *domain.OperationError
		if !errors.As(err, &opErr) {
			t.Fatalf("expected OperationError, got %T", err)
		}
		if opErr.Op != "unbind" {
			t.Errorf("Op = %q, want %q", opErr.Op, "unbind")
		}
	})
}

func TestBindDriver(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mut := &mockMutator{}
		svc := newTestService(mut)

		err := svc.BindDriver("0000:01:00.0", "nvidia")
		if err != nil {
			t.Fatalf("BindDriver() error = %v", err)
		}

		if len(mut.bindCalls) != 1 || mut.bindCalls[0].DriverName != "nvidia" {
			t.Errorf("expected Bind(0000:01:00.0, nvidia), got %v", mut.bindCalls)
		}
	})

	t.Run("error wraps as OperationError", func(t *testing.T) {
		mut := &mockMutator{bindErr: domain.ErrDeviceBusy}
		svc := newTestService(mut)

		err := svc.BindDriver("0000:01:00.0", "nvidia")
		if err == nil {
			t.Fatal("expected error")
		}

		var opErr *domain.OperationError
		if !errors.As(err, &opErr) {
			t.Fatalf("expected OperationError, got %T", err)
		}
		if opErr.Op != "bind" {
			t.Errorf("Op = %q, want %q", opErr.Op, "bind")
		}
	})
}

func TestRestoreDriver(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mut := &mockMutator{}
		svc := newTestService(mut)

		err := svc.RestoreDriver("0000:01:00.0", "pci")
		if err != nil {
			t.Fatalf("RestoreDriver() error = %v", err)
		}

		if len(mut.unbindCalls) != 1 {
			t.Errorf("expected 1 unbind call, got %d", len(mut.unbindCalls))
		}
		if len(mut.probeCalls) != 1 || mut.probeCalls[0] != "0000:01:00.0" {
			t.Errorf("expected DriverProbe(0000:01:00.0), got %v", mut.probeCalls)
		}
	})

	t.Run("unbind error ignored during restore", func(t *testing.T) {
		mut := &mockMutator{unbindErr: domain.ErrDriverNotBound}
		svc := newTestService(mut)

		err := svc.RestoreDriver("0000:01:00.0", "pci")
		if err != nil {
			t.Fatalf("RestoreDriver() should succeed even if already unbound, got %v", err)
		}

		if len(mut.probeCalls) != 1 {
			t.Errorf("expected DriverProbe to be called, got %d calls", len(mut.probeCalls))
		}
	})
}

func TestGetDeviceTree(t *testing.T) {
	t.Run("returns scanner tree", func(t *testing.T) {
		mut := &mockMutator{}
		svc := newTestService(mut)

		tree, err := svc.GetDeviceTree()
		if err != nil {
			t.Fatalf("GetDeviceTree() error = %v", err)
		}
		if tree.ID != "root" {
			t.Errorf("tree.ID = %q, want %q", tree.ID, "root")
		}
	})

	t.Run("scanner error propagates", func(t *testing.T) {
		scanErr := errors.New("scan failed")
		scanner := &mockScanner{err: scanErr}
		svc := NewDeviceService(scanner, &mockMutator{}, &mockListener{}, slog.Default())

		_, err := svc.GetDeviceTree()
		if !errors.Is(err, scanErr) {
			t.Errorf("expected scan error, got %v", err)
		}
	})
}

func TestHasMutator(t *testing.T) {
	t.Run("with mutator", func(t *testing.T) {
		svc := newTestService(&mockMutator{})
		if !svc.HasMutator() {
			t.Error("HasMutator() = false, want true")
		}
	})

	t.Run("without mutator", func(t *testing.T) {
		scanner := &mockScanner{tree: &domain.DeviceNode{ID: "root"}}
		svc := NewDeviceService(scanner, nil, &mockListener{}, slog.Default())
		if svc.HasMutator() {
			t.Error("HasMutator() = true, want false")
		}
	})
}
