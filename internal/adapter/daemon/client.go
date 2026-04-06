package daemon

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"defenestra.io/chassis/internal/core/port"
)

type Client struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

var _ port.DriverMutator = (*Client)(nil)

func NewClient() (*Client, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connecting to system bus: %w", err)
	}

	obj := conn.Object(dbusName, dbus.ObjectPath(dbusPath))

	return &Client{
		conn: conn,
		obj:  obj,
	}, nil
}

func IsAvailable() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		return false
	}

	obj := conn.Object(dbusName, dbus.ObjectPath(dbusPath))
	var reply string
	err = obj.Call(dbusInterface+".Ping", 0).Store(&reply)
	return err == nil
}

func (c *Client) Unbind(deviceID string, busType string) error {
	call := c.obj.Call(dbusInterface+".Unbind", 0, deviceID, busType)
	if call.Err != nil {
		return fmt.Errorf("unbind: %w", call.Err)
	}
	return nil
}

func (c *Client) Bind(deviceID string, driverName string) error {
	call := c.obj.Call(dbusInterface+".Bind", 0, deviceID, driverName)
	if call.Err != nil {
		return fmt.Errorf("bind: %w", call.Err)
	}
	return nil
}

func (c *Client) DriverProbe(deviceID string) error {
	call := c.obj.Call(dbusInterface+".DriverProbe", 0, deviceID)
	if call.Err != nil {
		return fmt.Errorf("driver probe: %w", call.Err)
	}
	return nil
}

func (c *Client) LoadModule(moduleName string) error {
	call := c.obj.Call(dbusInterface+".LoadModule", 0, moduleName)
	if call.Err != nil {
		return fmt.Errorf("load module: %w", call.Err)
	}
	return nil
}

func (c *Client) UnloadModule(moduleName string) error {
	call := c.obj.Call(dbusInterface+".UnloadModule", 0, moduleName)
	if call.Err != nil {
		return fmt.Errorf("unload module: %w", call.Err)
	}
	return nil
}

func (c *Client) Ping() error {
	var reply string
	err := c.obj.Call(dbusInterface+".Ping", 0).Store(&reply)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

func (c *Client) ReadSMART(blockDevice string) (string, error) {
	var jsonData string
	err := c.obj.Call(dbusInterface+".ReadSMART", 0, blockDevice).Store(&jsonData)
	if err != nil {
		return "", fmt.Errorf("read SMART: %w", err)
	}
	return jsonData, nil
}

func (c *Client) ReadSystemInfo() (string, error) {
	var jsonData string
	err := c.obj.Call(dbusInterface+".ReadSystemInfo", 0).Store(&jsonData)
	if err != nil {
		return "", fmt.Errorf("read system info: %w", err)
	}
	return jsonData, nil
}

func (c *Client) DumpVBIOS(pciAddress string) ([]byte, error) {
	var data []byte
	err := c.obj.Call(dbusInterface+".DumpVBIOS", 0, pciAddress).Store(&data)
	if err != nil {
		return nil, fmt.Errorf("dump VBIOS: %w", err)
	}
	return data, nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
