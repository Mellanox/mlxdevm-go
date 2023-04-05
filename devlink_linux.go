package mlxdevm

import (
	"bytes"
	"fmt"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
	"net"
	"syscall"
)

// DevlinkDevEswitchAttr represents device's eswitch attributes
type DevlinkDevEswitchAttr struct {
	Mode       string
	InlineMode string
	EncapMode  string
}

// DevlinkDevAttrs represents device attributes
type DevlinkDevAttrs struct {
	Eswitch DevlinkDevEswitchAttr
}

// DevlinkDevice represents device and its attributes
type DevlinkDevice struct {
	BusName    string
	DeviceName string
	Attrs      DevlinkDevAttrs
}

// DevlinkPortFn represents port function and its attributes
type DevlinkPortFn struct {
	HwAddr  net.HardwareAddr
	State   uint8
	OpState uint8
}

// DevlinkPortFnSetAttrs represents attributes to set
type DevlinkPortFnSetAttrs struct {
	FnAttrs     DevlinkPortFn
	HwAddrValid bool
	StateValid  bool
}

// DevlinkPortFnCap represents port function and its attributes
type DevlinkPortFnCap struct {
	Roce   bool
	UCList uint32
}

// DevlinkPortFnCapSetAttrs represents attributes to set
type DevlinkPortFnCapSetAttrs struct {
	FnCapAttrs  DevlinkPortFnCap
	RoceValid   bool
	UCListValid bool
}

// DevlinkDevParam represents a device parameter
type DevlinkDevParam struct {
	Name      string
	Attribute nl.Attribute
	CMode     uint8
}

// DevlinkPort represents port and its attributes
type DevlinkPort struct {
	BusName        string
	DeviceName     string
	PortIndex      uint32
	PortType       uint16
	NetdeviceName  string
	NetdevIfIndex  uint32
	RdmaDeviceName string
	PortFlavour    uint16
	Fn             *DevlinkPortFn
	PortCap        *DevlinkPortFnCap
}

type DevLinkPortAddAttrs struct {
	Controller      uint32
	SfNumber        uint32
	PortIndex       uint32
	PfNumber        uint16
	SfNumberValid   bool
	PortIndexValid  bool
	ControllerValid bool
}

var (
	native = nl.NativeEndian()
)

func parseDevLinkDeviceList(msgs [][]byte) ([]*DevlinkDevice, error) {
	devices := make([]*DevlinkDevice, 0, len(msgs))
	for _, m := range msgs {
		attrs, err := nl.ParseRouteAttr(m[nl.SizeofGenlmsg:])
		if err != nil {
			return nil, err
		}
		dev := &DevlinkDevice{}
		if err = dev.parseAttributes(attrs); err != nil {
			return nil, err
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func eswitchStringToMode(modeName string) (uint16, error) {
	if modeName == "legacy" {
		return DEVLINK_ESWITCH_MODE_LEGACY, nil
	} else if modeName == "switchdev" {
		return DEVLINK_ESWITCH_MODE_SWITCHDEV, nil
	} else {
		return 0xffff, fmt.Errorf("invalid switchdev mode")
	}
}

func parseEswitchMode(mode uint16) string {
	var eswitchMode = map[uint16]string{
		DEVLINK_ESWITCH_MODE_LEGACY:    "legacy",
		DEVLINK_ESWITCH_MODE_SWITCHDEV: "switchdev",
	}
	if eswitchMode[mode] == "" {
		return "unknown"
	} else {
		return eswitchMode[mode]
	}
}

func parseEswitchInlineMode(inlinemode uint8) string {
	var eswitchInlineMode = map[uint8]string{
		DEVLINK_ESWITCH_INLINE_MODE_NONE:      "none",
		DEVLINK_ESWITCH_INLINE_MODE_LINK:      "link",
		DEVLINK_ESWITCH_INLINE_MODE_NETWORK:   "network",
		DEVLINK_ESWITCH_INLINE_MODE_TRANSPORT: "transport",
	}
	if eswitchInlineMode[inlinemode] == "" {
		return "unknown"
	} else {
		return eswitchInlineMode[inlinemode]
	}
}

func parseEswitchEncapMode(encapmode uint8) string {
	var eswitchEncapMode = map[uint8]string{
		DEVLINK_ESWITCH_ENCAP_MODE_NONE:  "disable",
		DEVLINK_ESWITCH_ENCAP_MODE_BASIC: "enable",
	}
	if eswitchEncapMode[encapmode] == "" {
		return "unknown"
	} else {
		return eswitchEncapMode[encapmode]
	}
}

func (d *DevlinkDevice) parseAttributes(attrs []syscall.NetlinkRouteAttr) error {
	for _, a := range attrs {
		switch a.Attr.Type {
		case DEVLINK_ATTR_BUS_NAME:
			d.BusName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_DEV_NAME:
			d.DeviceName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_ESWITCH_MODE:
			d.Attrs.Eswitch.Mode = parseEswitchMode(native.Uint16(a.Value))
		case DEVLINK_ATTR_ESWITCH_INLINE_MODE:
			d.Attrs.Eswitch.InlineMode = parseEswitchInlineMode(uint8(a.Value[0]))
		case DEVLINK_ATTR_ESWITCH_ENCAP_MODE:
			d.Attrs.Eswitch.EncapMode = parseEswitchEncapMode(uint8(a.Value[0]))
		}
	}
	return nil
}

func (dev *DevlinkDevice) parseEswitchAttrs(msgs [][]byte) {
	m := msgs[0]
	attrs, err := nl.ParseRouteAttr(m[nl.SizeofGenlmsg:])
	if err != nil {
		return
	}
	dev.parseAttributes(attrs)
}

func (h *Handle) getEswitchAttrs(family *GenlFamily, dev *DevlinkDevice) {
	msg := &nl.Genlmsg{
		Command: DEVLINK_CMD_ESWITCH_GET,
		Version: nl.GENL_DEVLINK_VERSION,
	}
	req := h.newNetlinkRequest(int(family.ID), unix.NLM_F_REQUEST|unix.NLM_F_ACK)
	req.AddData(msg)

	b := make([]byte, len(dev.BusName))
	copy(b, dev.BusName)
	data := nl.NewRtAttr(DEVLINK_ATTR_BUS_NAME, b)
	req.AddData(data)

	b = make([]byte, len(dev.DeviceName))
	copy(b, dev.DeviceName)
	data = nl.NewRtAttr(DEVLINK_ATTR_DEV_NAME, b)
	req.AddData(data)

	msgs, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return
	}
	dev.parseEswitchAttrs(msgs)
}

// DevLinkGetDeviceList provides a pointer to devlink devices and nil error,
// otherwise returns an error code.
func (h *Handle) DevLinkGetDeviceList(Socket string) ([]*DevlinkDevice, error) {
	f, err := h.GenlFamilyGet(Socket)
	if err != nil {
		return nil, err
	}
	msg := &nl.Genlmsg{
		Command: DEVLINK_CMD_GET,
		Version: nl.GENL_DEVLINK_VERSION,
	}
	req := h.newNetlinkRequest(int(f.ID),
		unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_DUMP)
	req.AddData(msg)
	msgs, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}
	devices, err := parseDevLinkDeviceList(msgs)
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		h.getEswitchAttrs(f, d)
	}
	return devices, nil
}

// DevLinkGetDeviceList provides a pointer to devlink devices and nil error,
// otherwise returns an error code.
func DevLinkGetDeviceList(Socket string) ([]*DevlinkDevice, error) {
	return pkgHandle.DevLinkGetDeviceList(Socket)
}

func parseDevlinkDevice(msgs [][]byte) (*DevlinkDevice, error) {
	m := msgs[0]
	attrs, err := nl.ParseRouteAttr(m[nl.SizeofGenlmsg:])
	if err != nil {
		return nil, err
	}
	dev := &DevlinkDevice{}
	if err = dev.parseAttributes(attrs); err != nil {
		return nil, err
	}
	return dev, nil
}

func (h *Handle) createCmdReq(Socket string, cmd uint8, bus string, device string) (*GenlFamily, *nl.NetlinkRequest, error) {
	f, err := h.GenlFamilyGet(Socket)
	if err != nil {
		return nil, nil, err
	}

	msg := &nl.Genlmsg{
		Command: cmd,
		Version: nl.GENL_DEVLINK_VERSION,
	}
	req := h.newNetlinkRequest(int(f.ID),
		unix.NLM_F_REQUEST|unix.NLM_F_ACK)
	req.AddData(msg)

	b := make([]byte, len(bus)+1)
	copy(b, bus)
	data := nl.NewRtAttr(DEVLINK_ATTR_BUS_NAME, b)
	req.AddData(data)

	b = make([]byte, len(device)+1)
	copy(b, device)
	data = nl.NewRtAttr(DEVLINK_ATTR_DEV_NAME, b)
	req.AddData(data)

	return f, req, nil
}

// DevlinkGetDeviceByName provides a pointer to devlink device and nil error,
// otherwise returns an error code.
// Take Socket as either GENL_DEVLINK_NAME or as GENL_MLXDEVM_NAME.
func (h *Handle) DevLinkGetDeviceByName(Socket string, Bus string, Device string) (*DevlinkDevice, error) {
	f, req, err := h.createCmdReq(Socket, DEVLINK_CMD_GET, Bus, Device)
	if err != nil {
		return nil, err
	}

	respmsg, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}
	dev, err := parseDevlinkDevice(respmsg)
	if err == nil {
		h.getEswitchAttrs(f, dev)
	}
	return dev, err
}

// DevlinkGetDeviceByName provides a pointer to devlink device and nil error,
// otherwise returns an error code.
// Take Socket as either GENL_DEVLINK_NAME or as GENL_MLXDEVM_NAME.
func DevLinkGetDeviceByName(Socket string, Bus string, Device string) (*DevlinkDevice, error) {
	return pkgHandle.DevLinkGetDeviceByName(Socket, Bus, Device)
}

// DevLinkSetEswitchMode sets eswitch mode if able to set successfully or
// returns an error code.
// Equivalent to: `devlink dev eswitch set $dev mode switchdev`
// Equivalent to: `devlink dev eswitch set $dev mode legacy`
func (h *Handle) DevLinkSetEswitchMode(Socket string, Dev *DevlinkDevice, NewMode string) error {
	mode, err := eswitchStringToMode(NewMode)
	if err != nil {
		return err
	}

	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_ESWITCH_SET, Dev.BusName, Dev.DeviceName)
	if err != nil {
		return err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_ESWITCH_MODE, nl.Uint16Attr(mode)))

	_, err = req.Execute(unix.NETLINK_GENERIC, 0)
	return err
}

// DevLinkSetEswitchMode sets eswitch mode if able to set successfully or
// returns an error code.
// Equivalent to: `devlink dev eswitch set $dev mode switchdev`
// Equivalent to: `devlink dev eswitch set $dev mode legacy`
func DevLinkSetEswitchMode(Socket string, Dev *DevlinkDevice, NewMode string) error {
	return pkgHandle.DevLinkSetEswitchMode(Socket, Dev, NewMode)
}

func (port *DevlinkPort) parseAttributes(attrs []syscall.NetlinkRouteAttr) error {
	for _, a := range attrs {
		switch a.Attr.Type {
		case DEVLINK_ATTR_BUS_NAME:
			port.BusName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_DEV_NAME:
			port.DeviceName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_PORT_INDEX:
			port.PortIndex = native.Uint32(a.Value)
		case DEVLINK_ATTR_PORT_TYPE:
			port.PortType = native.Uint16(a.Value)
		case DEVLINK_ATTR_PORT_NETDEV_NAME:
			port.NetdeviceName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_PORT_NETDEV_IFINDEX:
			port.NetdevIfIndex = native.Uint32(a.Value)
		case DEVLINK_ATTR_PORT_IBDEV_NAME:
			port.RdmaDeviceName = string(a.Value[:len(a.Value)-1])
		case DEVLINK_ATTR_PORT_FLAVOUR:
			port.PortFlavour = native.Uint16(a.Value)
		default:
			// All nested attributes are located together
			if a.Attr.Type&unix.NLA_F_NESTED != 0 {
				for nested := range nl.ParseAttributes(a.Value) {
					switch nested.Type {
					case DEVLINK_PORT_FUNCTION_ATTR_HW_ADDR:
						if port.Fn == nil {
							port.Fn = &DevlinkPortFn{}
						}
						port.Fn.HwAddr = nested.Value[:]
					case DEVLINK_PORT_FN_ATTR_STATE:
						if port.Fn == nil {
							port.Fn = &DevlinkPortFn{}
						}
						port.Fn.State = uint8(nested.Value[0])
					case DEVLINK_PORT_FN_ATTR_OPSTATE:
						if port.Fn == nil {
							port.Fn = &DevlinkPortFn{}
						}
						port.Fn.OpState = uint8(nested.Value[0])
					case DEVLINK_PORT_FN_ATTR_EXT_CAP_ROCE:
						if port.PortCap == nil {
							port.PortCap = &DevlinkPortFnCap{}
						}
						port.PortCap.Roce = uint8(nested.Value[0]) != 0
					case DEVLINK_PORT_FN_ATTR_EXT_CAP_UC_LIST:
						if port.PortCap == nil {
							port.PortCap = &DevlinkPortFnCap{}
						}
						port.PortCap.UCList = uint32(nested.Value[0])
					}
				}
			}
		}
	}
	return nil
}

func parseDevLinkAllPortList(msgs [][]byte) ([]*DevlinkPort, error) {
	ports := make([]*DevlinkPort, 0, len(msgs))
	for _, m := range msgs {
		attrs, err := nl.ParseRouteAttr(m[nl.SizeofGenlmsg:])
		if err != nil {
			return nil, err
		}
		port := &DevlinkPort{}
		if err = port.parseAttributes(attrs); err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, nil
}

// DevLinkGetPortList provides a pointer to devlink ports and nil error,
// otherwise returns an error code.
func (h *Handle) DevLinkGetAllPortList(Socket string) ([]*DevlinkPort, error) {
	f, err := h.GenlFamilyGet(Socket)
	if err != nil {
		return nil, err
	}
	msg := &nl.Genlmsg{
		Command: DEVLINK_CMD_PORT_GET,
		Version: nl.GENL_DEVLINK_VERSION,
	}
	req := h.newNetlinkRequest(int(f.ID),
		unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_DUMP)
	req.AddData(msg)
	msgs, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}
	ports, err := parseDevLinkAllPortList(msgs)
	if err != nil {
		return nil, err
	}
	return ports, nil
}

// DevLinkGetPortList provides a pointer to devlink ports and nil error,
// otherwise returns an error code.
func DevLinkGetAllPortList(Socket string) ([]*DevlinkPort, error) {
	return pkgHandle.DevLinkGetAllPortList(Socket)
}

func parseDevlinkPortMsg(msgs [][]byte) (*DevlinkPort, error) {
	m := msgs[0]
	attrs, err := nl.ParseRouteAttr(m[nl.SizeofGenlmsg:])
	if err != nil {
		return nil, err
	}
	port := &DevlinkPort{}
	if err = port.parseAttributes(attrs); err != nil {
		return nil, err
	}
	return port, nil
}

// DevLinkGetPortByIndexprovides a pointer to devlink device and nil error,
// otherwise returns an error code.
func (h *Handle) DevLinkGetPortByIndex(Socket string, Bus string, Device string, PortIndex uint32) (*DevlinkPort, error) {

	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_PORT_GET, Bus, Device)
	if err != nil {
		return nil, err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_INDEX, nl.Uint32Attr(PortIndex)))

	respmsg, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}
	port, err := parseDevlinkPortMsg(respmsg)
	return port, err
}

// DevLinkGetPortByIndex provides a pointer to devlink portand nil error,
// otherwise returns an error code.
func DevLinkGetPortByIndex(Socket string, Bus string, Device string, PortIndex uint32) (*DevlinkPort, error) {
	return pkgHandle.DevLinkGetPortByIndex(Socket, Bus, Device, PortIndex)
}

// DevLinkPortAdd adds a devlink port and returns a port on success
// otherwise returns nil port and an error code.
func (h *Handle) DevLinkPortAdd(Socket string, Bus string, Device string, Flavour uint16, Attrs DevLinkPortAddAttrs) (*DevlinkPort, error) {
	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_PORT_NEW, Bus, Device)
	if err != nil {
		return nil, err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_FLAVOUR, nl.Uint16Attr(Flavour)))

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_PCI_PF_NUMBER, nl.Uint16Attr(Attrs.PfNumber)))
	if Flavour == DEVLINK_PORT_FLAVOUR_PCI_SF && Attrs.SfNumberValid {
		req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_PCI_SF_NUMBER, nl.Uint32Attr(Attrs.SfNumber)))
	}
	if Attrs.PortIndexValid {
		req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_INDEX, nl.Uint32Attr(Attrs.PortIndex)))
	}
	/*
		if Attrs.ControllerValid {
			req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_CONTROLLER_NUMBER, nl.Uint32Attr(Attrs.Controller)))
		}
	*/
	respmsg, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}
	port, err := parseDevlinkPortMsg(respmsg)
	return port, err
}

// DevLinkPortAdd adds a devlink port and returns a port on success
// otherwise returns nil port and an error code.
func DevLinkPortAdd(Socket string, Bus string, Device string, Flavour uint16, Attrs DevLinkPortAddAttrs) (*DevlinkPort, error) {
	return pkgHandle.DevLinkPortAdd(Socket, Bus, Device, Flavour, Attrs)
}

// DevLinkPortDel deletes a devlink port and returns success or error code.
func (h *Handle) DevLinkPortDel(Socket string, Bus string, Device string, PortIndex uint32) error {
	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_PORT_DEL, Bus, Device)
	if err != nil {
		return err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_INDEX, nl.Uint32Attr(PortIndex)))
	_, err = req.Execute(unix.NETLINK_GENERIC, 0)
	return err
}

// DevLinkPortDel deletes a devlink port and returns success or error code.
func DevLinkPortDel(Socket string, Bus string, Device string, PortIndex uint32) error {
	return pkgHandle.DevLinkPortDel(Socket, Bus, Device, PortIndex)
}

// DevlinkPortFnSet sets one or more port function attributes specified by the attribute mask.
// It returns 0 on success or error code.
func (h *Handle) DevlinkPortFnSet(Socket string, Bus string, Device string, PortIndex uint32, FnAttrs DevlinkPortFnSetAttrs) error {
	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_PORT_SET, Bus, Device)
	if err != nil {
		return err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_INDEX, nl.Uint32Attr(PortIndex)))

	fnAttr := nl.NewRtAttr(DEVLINK_ATTR_PORT_FUNCTION|unix.NLA_F_NESTED, nil)

	if FnAttrs.HwAddrValid == true {
		fnAttr.AddRtAttr(DEVLINK_PORT_FUNCTION_ATTR_HW_ADDR, []byte(FnAttrs.FnAttrs.HwAddr))
	}

	if FnAttrs.StateValid == true {
		fnAttr.AddRtAttr(DEVLINK_PORT_FN_ATTR_STATE, nl.Uint8Attr(FnAttrs.FnAttrs.State))
	}
	req.AddData(fnAttr)

	_, err = req.Execute(unix.NETLINK_GENERIC, 0)
	return err
}

// DevlinkPortFnSet sets one or more port function attributes specified by the attribute mask.
// It returns 0 on success or error code.
func DevlinkPortFnSet(Socket string, Bus string, Device string, PortIndex uint32, FnAttrs DevlinkPortFnSetAttrs) error {
	return pkgHandle.DevlinkPortFnSet(Socket, Bus, Device, PortIndex, FnAttrs)
}

// DevlinkPortFnCapSet sets roce and max_uc_macs port function cap attributes.
// It returns 0 on success or error code.
// Equivalent to: `mlxdevm port function cap sep $port roce true max_uc_macs 64`
// Equivalent to: `mlxdevm port function cap sep $port roce false max_uc_macs 128`
func (h *Handle) DevlinkPortFnCapSet(Socket string, Bus string, Device string, PortIndex uint32, FnCapAttrs DevlinkPortFnCapSetAttrs) error {
	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_EXT_CAP_SET, Bus, Device)
	if err != nil {
		return err
	}

	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PORT_INDEX, nl.Uint32Attr(PortIndex)))

	fnAttr := nl.NewRtAttr(DEVLINK_ATTR_EXT_PORT_FN_CAP|unix.NLA_F_NESTED, nil)

	if FnCapAttrs.RoceValid {
		roce := uint8(0)
		if FnCapAttrs.FnCapAttrs.Roce {
			roce = 1
		}
		fnAttr.AddRtAttr(DEVLINK_PORT_FN_ATTR_EXT_CAP_ROCE, nl.Uint8Attr(roce))
	}

	if FnCapAttrs.UCListValid == true {
		fnAttr.AddRtAttr(DEVLINK_PORT_FN_ATTR_EXT_CAP_UC_LIST, nl.Uint32Attr(FnCapAttrs.FnCapAttrs.UCList))
	}
	req.AddData(fnAttr)

	_, err = req.Execute(unix.NETLINK_GENERIC, 0)
	return err
}

// DevlinkPortFnCapSet sets roce and max_uc_macs port function cap attributes.
// It returns 0 on success or error code.
// Equivalent to: `mlxdevm port function cap sep $port roce true max_uc_macs 64`
// Equivalent to: `mlxdevm port function cap sep $port roce false max_uc_macs 128`
func DevlinkPortFnCapSet(Socket string, Bus string, Device string, PortIndex uint32, FnCapAttrs DevlinkPortFnCapSetAttrs) error {
	return pkgHandle.DevlinkPortFnCapSet(Socket, Bus, Device, PortIndex, FnCapAttrs)
}

func parseDevParam(data []byte) *DevlinkDevParam {
	param := DevlinkDevParam{}
	var stack [][]byte
	stack = append(stack, data)

	for len(stack) > 0 {
		data = stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for a := range nl.ParseAttributes(data) {
			switch a.Type {
			case DEVLINK_ATTR_PARAM_NAME:
				param.Name = string(a.Value[:len(a.Value)-1])
			case DEVLINK_ATTR_PARAM_TYPE:
				param.Attribute.Type = uint16(a.Value[0])
			case DEVLINK_ATTR_PARAM_VALUE_CMODE:
				param.CMode = a.Value[0]
			case DEVLINK_ATTR_PARAM_VALUE_DATA:
				param.Attribute.Value = a.Value
			case DEVLINK_ATTR_PARAM_VALUE | unix.NLA_F_NESTED:
				if param.Attribute.Type == MNL_TYPE_FLAG {
					value := 0
					if bytes.Contains(a.Value, []byte{4, 0, DEVLINK_ATTR_PARAM_VALUE_DATA, 0}) {
						value = 1
					}
					param.Attribute.Value = []byte{uint8(value)}
				}
			}
			if a.Type&unix.NLA_F_NESTED != 0 {
				stack = append(stack, a.Value)
			}
		}
	}

	return &param
}

// DevlinkDevParamGet returns information about a set device parameter
// Equivalent to `mlxdevm dev param show $dev name disable_netdev`
func (h *Handle) DevlinkDevParamGet(Socket string, Bus string, Device string, ParamName string) (*DevlinkDevParam, error) {
	_, req, err := h.createCmdReq(Socket, DEVLINK_CMD_PARAM_GET, Bus, Device)
	if err != nil {
		return nil, err
	}

	b := make([]byte, len(ParamName)+1)
	copy(b, ParamName)
	req.AddData(nl.NewRtAttr(DEVLINK_ATTR_PARAM_NAME, b))

	respmsg, err := req.Execute(unix.NETLINK_GENERIC, 0)
	if err != nil {
		return nil, err
	}

	return parseDevParam(respmsg[0][nl.SizeofGenlmsg:]), nil
}

// DevlinkDevParamGet returns information about a set device parameter
// Equivalent to `mlxdevm dev param show $dev name disable_netdev`
func DevlinkDevParamGet(Socket string, Bus string, Device string, ParamName string) (*DevlinkDevParam, error) {
	return pkgHandle.DevlinkDevParamGet(Socket, Bus, Device, ParamName)
}
