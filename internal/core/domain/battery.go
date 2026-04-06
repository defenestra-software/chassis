package domain

type BatteryInfo struct {
	Status        string
	Capacity      int // -1 if unavailable
	CapacityLevel string
	Model         string
	Manufacturer  string
}
