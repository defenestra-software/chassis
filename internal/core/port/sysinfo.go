package port

import "defenestra.io/chassis/internal/core/domain"

type SysInfoScanner interface {
	ScanProcessors() []*domain.DeviceNode
	ScanSensors() []*domain.DeviceNode
	ScanDisplays() []*domain.DeviceNode
	ScanPower() []*domain.DeviceNode
	ScanMotherboard() *domain.DeviceNode
	ScanMemory() []*domain.DeviceNode
	ScanBluetoothHID() []*domain.DeviceNode
	EnrichCameraInfo(root *domain.DeviceNode)
	EnrichSerialPorts(root *domain.DeviceNode)
	EnrichNetworkInfo(root *domain.DeviceNode)
	EnrichPeripheralBatteries(root *domain.DeviceNode)
	EnrichUPSBattery(root *domain.DeviceNode)
	EnrichUSBStorage(root *domain.DeviceNode)
}
