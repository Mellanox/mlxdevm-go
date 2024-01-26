//go:build linux
// +build linux

package mlxdevm

import (
	"errors"
	"flag"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestDevlinkGetDeviceList(t *testing.T) {
	_, err := DevlinkGetDeviceList(socket)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevlinkGetDeviceByName(t *testing.T) {
	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DevlinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevlinkGetAllPortList(t *testing.T) {
	ports, err := DevlinkGetAllPortList(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("devlink port count = ", len(ports))
	for _, port := range ports {
		t.Logf("Port: %+v", *port)
		if port.Fn != nil {
			t.Logf("Port Function: %+v", *port.Fn)
		}
		if port.PortCap != nil {
			t.Logf("Port Function Cap: %+v", *port.PortCap)
		}
	}
}

func TestDevlinkAddDelSfPort(t *testing.T) {
	var addAttrs DevlinkPortAddAttrs
	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := DevlinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}
	addAttrs.SfNumberValid = true
	addAttrs.SfNumber = uint32(sfnum)
	addAttrs.PfNumber = uint16(pfnum)
	port, err2 := DevlinkPortAdd(socket, dev.BusName, dev.DeviceName, 7, addAttrs)
	if err2 != nil {
		t.Fatal(err2)
		return
	}
	t.Logf("Port: %+v", *port)
	if port.Fn != nil {
		t.Logf("Port Function Attributes: %+v", *port.Fn)
	}

	if port.PortCap != nil {
		t.Logf("Port Function Cap: %+v", *port.PortCap)
	}

	assert := assert.New(t)
	assert.Equal(uint32(0), port.Controller, "miss matching controller number")
	assert.Equal(uint16(0), port.PfNumber, "miss-matching PF number")
	assert.Equal(addAttrs.SfNumber, port.SfNumber, "miss-matching SF number")

	err2 = DevlinkPortDel(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err2 != nil {
		t.Fatal(err2)
	}
}

func TestDevlinkSfPortFnSet(t *testing.T) {
	var addAttrs DevlinkPortAddAttrs
	var stateAttr DevlinkPortFnSetAttrs

	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := DevlinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}
	addAttrs.SfNumberValid = true
	addAttrs.SfNumber = uint32(sfnum)
	addAttrs.PfNumber = uint16(pfnum)
	port, err2 := DevlinkPortAdd(socket, dev.BusName, dev.DeviceName, 7, addAttrs)
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

	stateAttr.FnAttrs.Trust = 1
	stateAttr.TrustValid = true
	err2 = DevlinkPortFnSet(socket, dev.BusName, dev.DeviceName, port.PortIndex, stateAttr)
	if err2 != nil {
		t.Fatal("function trust set err = ", err2)
	}

	port, err3 := DevlinkGetPortByIndex(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err3 == nil {
		t.Log(*port)
		t.Log(*port.Fn)
	}
	err2 = DevlinkPortDel(socket, dev.BusName, dev.DeviceName, port.PortIndex)
	if err2 != nil {
		t.Fatal(err2)
	}
}

func TestDevlinkPortFnCapSet(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test TestDevlinkPortFnCapSet in CI environment until test is fixed")
	}

	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}

	dev, err := DevlinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}

	portIndex := uint32(0)

	testCases := []struct {
		name        string
		fnCapAttrs  DevlinkPortFnCapSetAttrs
		errExpected bool
	}{
		{
			name: "Roce true, max_uc_macs 64",
			fnCapAttrs: DevlinkPortFnCapSetAttrs{
				RoceValid:   true,
				FnCapAttrs:  DevlinkPortFnCap{Roce: true, UCList: 64},
				UCListValid: true,
			},
			errExpected: false,
		},
		{
			name: "Roce false, max_uc_macs 128",
			fnCapAttrs: DevlinkPortFnCapSetAttrs{
				RoceValid:   true,
				FnCapAttrs:  DevlinkPortFnCap{Roce: false, UCList: 128},
				UCListValid: true,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := pkgHandle.DevlinkPortFnCapSet(socket, dev.BusName, dev.DeviceName, portIndex, tc.fnCapAttrs)
			if (err != nil) != tc.errExpected {
				t.Fatalf("Expected error: %v, got: %v", tc.errExpected, err)
			}
		})
	}
}

func TestDevlinkDevParamSet(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test TestDevlinkDevParamSet in CI environment until test is fixed")
	}

	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}

	dev, err := DevlinkGetDeviceByName(socket, bus, device)
	if err != nil {
		t.Fatal(err)
		return
	}

	testCases := []struct {
		name        string
		paramName   string
		newValue    string
		newCMode    string
		errExpected bool
	}{
		{
			name:        "Set disable_netdev true, cmode runtime",
			paramName:   "disable_netdev",
			newValue:    "true",
			newCMode:    "runtime",
			errExpected: false,
		},
		{
			name:        "Set disable_netdev false, cmode runtime",
			paramName:   "disable_netdev",
			newValue:    "false",
			newCMode:    "runtime",
			errExpected: false,
		},
		{
			name:        "Set disable_netdev false, cmode runtime",
			paramName:   "disable_netdev",
			newValue:    "arbitrary value",
			newCMode:    "runtime",
			errExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := pkgHandle.DevlinkDevParamSet(socket, dev.BusName, dev.DeviceName, tc.paramName, tc.newValue, tc.newCMode)
			if (err != nil) != tc.errExpected {
				t.Fatalf("Expected error: %v, got: %v", tc.errExpected, err)
			}
		})
	}
}

func TestDevlinkGetDeviceResources(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test TestDevlinkDevParamSet in CI environment until test is fixed")
	}

	err := validateArgs(t)
	if err != nil {
		t.Fatal(err)
	}

	res, err := DevlinkGetDeviceResources(socket, bus, device)
	if err != nil {
		t.Fatalf("failed to get device(%s: %s/%s) resources. %s", socket, bus, device, err)
	}

	if res.Bus != bus || res.Device != device {
		t.Fatalf("missmatching bus/device")
	}

	t.Logf("Resources: %+v", res)
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
