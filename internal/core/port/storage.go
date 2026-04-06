package port

import (
	"time"

	"defenestra.io/chassis/internal/core/domain"
)

type StorageScanner interface {
	ScanStorage() ([]*domain.StorageInfo, error)

	GetStorageByPCI(pciAddress string) *domain.StorageInfo

	StartMonitor(interval time.Duration, onChange func())

	Stop()

	Refresh()
}
