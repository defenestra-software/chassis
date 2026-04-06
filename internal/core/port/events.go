package port

import "defenestra.io/chassis/internal/core/domain"

type EventListener interface {
	Subscribe() (<-chan domain.HardwareEvent, error)

	Stop() error
}
