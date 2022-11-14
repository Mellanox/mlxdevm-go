// +build linux

package mlxdevm

import (
	"errors"
	"flag"
	"net"
	"testing"
)

func validateArgs(t *testing.T) error {
	if socket == "" {
		t.Log("user must specify socket name as devlink or mlxdevm")
		return errors.New("empty socket name")
	}
	if socket != "mlxdevm" && socket != "devlink" {
		t.Log("user must specify socket name as devlink or mlxdevm")
		return errors.New("empty socket name")
	}
	if bus == "" || device == "" {
		t.Log("devlink bus and device are empty, skipping test")
		return errors.New("empty socket name")
	}
	return nil
}

func TestDevLinkGetDeviceList(t *testing.T) {
	_, err := DevLinkGetDeviceList(socket)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevLinkGetDeviceByName(t *testing.T) {
	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DevLinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevLinkGetAllPortList(t *testing.T) {
	ports, err := DevLinkGetAllPortList(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("devlink port count = ", len(ports))
	for _, port := range ports {
		t.Log(*port)
	}
}

func TestDevLinkAddDelSfPort(t *testing.T) {
	var addAttrs DevLinkPortAddAttrs
	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := DevLinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}
	addAttrs.SfNumberValid = true
	addAttrs.SfNumber = uint32(sfnum)
	addAttrs.PfNumber = 0
	port, err2 := DevLinkPortAdd(socket, dev.BusName, dev.DeviceName, 7, addAttrs)
	if err2 != nil {
		t.Fatal(err2)
		return
	}
	t.Log(*port)
	if port.Fn != nil {
		t.Log("function attributes = ", *port.Fn)
	}
	err2 = DevLinkPortDel(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err2 != nil {
		t.Fatal(err2)
	}
}

func TestDevLinkSfPortFnSet(t *testing.T) {
	var addAttrs DevLinkPortAddAttrs
	var stateAttr DevlinkPortFnSetAttrs

	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := DevLinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}
	addAttrs.SfNumberValid = true
	addAttrs.SfNumber = uint32(sfnum)
	addAttrs.PfNumber = 0
	port, err2 := DevLinkPortAdd(socket, dev.BusName, dev.DeviceName, 7, addAttrs)
	if err2 != nil {
		t.Fatal(err2)
		return
	}
	t.Log(*port)
	if port.Fn != nil {
		t.Log("function attributes = ", *port.Fn)
	}
	macAttr := DevlinkPortFnSetAttrs{
		FnAttrs: DevlinkPortFn{
			HwAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		},
		HwAddrValid: true,
	}
	err2 = DevlinkPortFnSet(socket, dev.BusName, dev.DeviceName, port.PortIndex, macAttr)
	if err2 != nil {
		t.Log("function mac set err = ", err2)
	}
	stateAttr.FnAttrs.State = 1
	stateAttr.StateValid = true
	err2 = DevlinkPortFnSet(socket, dev.BusName, dev.DeviceName, port.PortIndex, stateAttr)
	if err2 != nil {
		t.Log("function state set err = ", err2)
	}

	port, err3 := DevLinkGetPortByIndex(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err3 == nil {
		t.Log(*port)
		t.Log(*port.Fn)
	}
	err2 = DevLinkPortDel(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err2 != nil {
		t.Fatal(err2)
	}
}

var socket string
var bus string
var device string
var sfnum uint
var pfnum uint

func init() {
	flag.StringVar(&socket, "socketname", "mlxdevm", "socket name as devlink or mlxdevm")
	flag.StringVar(&bus, "bus", "", "devlink device bus name")
	flag.StringVar(&device, "device", "", "devlink device devicename")
	flag.UintVar(&pfnum, "pfnum", 0, "devlink port pfnumber")
	flag.UintVar(&sfnum, "sfnum", 0, "devlink port sfnumber")
}
