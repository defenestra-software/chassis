package port

import "defenestra.io/chassis/internal/core/domain"

type ActionHandler func(deviceID string, action string) error

type UIAdapter interface {
	Run() error

	ShowDeviceTree(root *domain.DeviceNode)
	ShowError(err error)
	ShowProgress(message string)
	HideProgress()
	RefreshDevice(device *domain.DeviceNode)
	SetActionHandler(handler ActionHandler)
	SetRefreshHandler(handler func() error)
	SetDriverManagementEnabled(enabled bool)
}
