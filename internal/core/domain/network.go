package domain

type NetworkInfo struct {
	InterfaceName string
	MACAddress    string
	Speed         string
	Duplex        string
	MTU           string
	State         string
	IsWireless    bool
	IsBluetooth   bool
	IsVirtual     bool
}
