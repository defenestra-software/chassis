package port

import "defenestra.io/chassis/internal/core/domain"

type DeviceScanner interface {
	Scan() (*domain.DeviceNode, error)
	GetDevice(id string) (*domain.DeviceNode, error)
	GetDevicesByType(deviceType domain.DeviceType) ([]*domain.DeviceNode, error)
	GetIOMMUGroup(group int) ([]*domain.DeviceNode, error)
}

type GPUScanner interface {
	ScanGPUs() ([]*domain.GPUInfo, error)
	GetGPUByPCI(pciAddress string) *domain.GPUInfo
}
