// This file was copied from https://github.com/vishvananda/netlink with
// Changelog which differs from upstream version
// 	NetlinkRequest.Execute() : dont fail if NLMSG_DONE contains no data, assume
//							   no error

// Package nl has low level primitives for making Netlink calls.
// nolint
package nl

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const (
	// Arbitrary set value (greater than default 4k) to allow receiving
	// from kernel more verbose messages e.g. for statistics,
	// tc rules or filters, or other more memory requiring data.
	RECEIVE_BUFFER_SIZE = 65536
	// Kernel netlink pid
	PidKernel uint32 = 0
)

// SupportedNlFamilies contains the list of netlink families this netlink package supports
var SupportedNlFamilies = []int{unix.NETLINK_ROUTE, unix.NETLINK_XFRM, unix.NETLINK_NETFILTER}

var nextSeqNr uint32

// Default netlink socket timeout, 60s
var SocketTimeoutTv = unix.Timeval{Sec: 60, Usec: 0}

// ErrorMessageReporting is the default error message reporting configuration for the new netlink sockets
var EnableErrorMessageReporting bool = false

var nativeEndian binary.ByteOrder

// NativeEndian gets native endianness for the system
func NativeEndian() binary.ByteOrder {
	if nativeEndian == nil {
		var x uint32 = 0x01020304
		if *(*byte)(unsafe.Pointer(&x)) == 0x01 {
			nativeEndian = binary.BigEndian
		} else {
			nativeEndian = binary.LittleEndian
		}
	}
	return nativeEndian
}

const (
	NLMSGERR_ATTR_UNUSED = 0
	NLMSGERR_ATTR_MSG    = 1
	NLMSGERR_ATTR_OFFS   = 2
	NLMSGERR_ATTR_COOKIE = 3
	NLMSGERR_ATTR_POLICY = 4
)

type NetlinkRequestData interface {
	Len() int
	Serialize() []byte
}

// Round the length of a netlink message up to align it properly.
// Taken from syscall/netlink_linux.go by The Go Authors under BSD-style license.
func nlmAlignOf(msglen int) int {
	return (msglen + syscall.NLMSG_ALIGNTO - 1) & ^(syscall.NLMSG_ALIGNTO - 1)
}

func rtaAlignOf(attrlen int) int {
	return (attrlen + unix.RTA_ALIGNTO - 1) & ^(unix.RTA_ALIGNTO - 1)
}

type NetlinkRequest struct {
	unix.NlMsghdr
	Data    []NetlinkRequestData
	RawData []byte
	Sockets map[int]*SocketHandle
}

// Serialize the Netlink Request into a byte array
func (req *NetlinkRequest) Serialize() []byte {
	length := unix.SizeofNlMsghdr
	dataBytes := make([][]byte, len(req.Data))
	for i, data := range req.Data {
		dataBytes[i] = data.Serialize()
		length = length + len(dataBytes[i])
	}
	length += len(req.RawData)

	req.Len = uint32(length)
	b := make([]byte, length)
	hdr := (*(*[unix.SizeofNlMsghdr]byte)(unsafe.Pointer(req)))[:]
	next := unix.SizeofNlMsghdr
	copy(b[0:next], hdr)
	for _, data := range dataBytes {
		for _, dataByte := range data {
			b[next] = dataByte
			next = next + 1
		}
	}
	// Add the raw data if any
	if len(req.RawData) > 0 {
		copy(b[next:length], req.RawData)
	}
	return b
}

func (req *NetlinkRequest) AddData(data NetlinkRequestData) {
	req.Data = append(req.Data, data)
}

// AddRawData adds raw bytes to the end of the NetlinkRequest object during serialization
func (req *NetlinkRequest) AddRawData(data []byte) {
	req.RawData = append(req.RawData, data...)
}

// Execute the request against a the given sockType.
// Returns a list of netlink messages in serialized format, optionally filtered
// by resType.
func (req *NetlinkRequest) Execute(sockType int, resType uint16) ([][]byte, error) {
	var (
		s   *NetlinkSocket
		err error
	)

	if req.Sockets != nil {
		if sh, ok := req.Sockets[sockType]; ok {
			s = sh.Socket
			req.Seq = atomic.AddUint32(&sh.Seq, 1)
		}
	}
	sharedSocket := s != nil

	if s == nil {
		s, err = getNetlinkSocket(sockType)
		if err != nil {
			return nil, err
		}

		if err := s.SetSendTimeout(&SocketTimeoutTv); err != nil {
			return nil, err
		}
		if err := s.SetReceiveTimeout(&SocketTimeoutTv); err != nil {
			return nil, err
		}
		if EnableErrorMessageReporting {
			if err := s.SetExtAck(true); err != nil {
				return nil, err
			}
		}

		defer s.Close()
	} else {
		s.Lock()
		defer s.Unlock()
	}

	if err := s.Send(req); err != nil {
		return nil, err
	}

	pid, err := s.GetPid()
	if err != nil {
		return nil, err
	}

	var res [][]byte

done:
	for {
		msgs, from, err := s.Receive()
		if err != nil {
			return nil, err
		}
		if from.Pid != PidKernel {
			return nil, fmt.Errorf("Wrong sender portid %d, expected %d", from.Pid, PidKernel)
		}
		for _, m := range msgs {
			if m.Header.Seq != req.Seq {
				if sharedSocket {
					continue
				}
				return nil, fmt.Errorf("Wrong Seq nr %d, expected %d", m.Header.Seq, req.Seq)
			}
			if m.Header.Pid != pid {
				continue
			}

			if m.Header.Flags&unix.NLM_F_DUMP_INTR != 0 {
				return nil, syscall.Errno(unix.EINTR)
			}

			if m.Header.Type == unix.NLMSG_DONE || m.Header.Type == unix.NLMSG_ERROR {
				// Note(Adrianc): This is a WA to handle bug in kernel where on Dump Resource command
				// it does not provide error field in NLMSG_DONE.
				if len(m.Data) == 0 {
					break done
				}

				native := NativeEndian()
				errno := int32(native.Uint32(m.Data[0:4]))
				if errno == 0 {
					break done
				}
				var err error
				err = syscall.Errno(-errno)

				unreadData := m.Data[4:]
				if m.Header.Flags&unix.NLM_F_ACK_TLVS != 0 && len(unreadData) > syscall.SizeofNlMsghdr {
					// Skip the echoed request message.
					echoReqH := (*syscall.NlMsghdr)(unsafe.Pointer(&unreadData[0]))
					unreadData = unreadData[nlmAlignOf(int(echoReqH.Len)):]

					// Annotate `err` using nlmsgerr attributes.
					for len(unreadData) >= syscall.SizeofRtAttr {
						attr := (*syscall.RtAttr)(unsafe.Pointer(&unreadData[0]))
						attrData := unreadData[syscall.SizeofRtAttr:attr.Len]

						switch attr.Type {
						case NLMSGERR_ATTR_MSG:
							err = fmt.Errorf("%w: %s", err, unix.ByteSliceToString(attrData))
						default:
							// TODO: handle other NLMSGERR_ATTR types
						}

						unreadData = unreadData[rtaAlignOf(int(attr.Len)):]
					}
				}

				return nil, err
			}
			if resType != 0 && m.Header.Type != resType {
				continue
			}
			res = append(res, m.Data)
			if m.Header.Flags&unix.NLM_F_MULTI == 0 {
				break done
			}
		}
	}
	return res, nil
}

// Create a new netlink request from proto and flags
// Note the Len value will be inaccurate once data is added until
// the message is serialized
func NewNetlinkRequest(proto, flags int) *NetlinkRequest {
	return &NetlinkRequest{
		NlMsghdr: unix.NlMsghdr{
			Len:   uint32(unix.SizeofNlMsghdr),
			Type:  uint16(proto),
			Flags: unix.NLM_F_REQUEST | uint16(flags),
			Seq:   atomic.AddUint32(&nextSeqNr, 1),
		},
	}
}

type NetlinkSocket struct {
	fd  int32
	lsa unix.SockaddrNetlink
	sync.Mutex
}

func getNetlinkSocket(protocol int) (*NetlinkSocket, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, protocol)
	if err != nil {
		return nil, err
	}
	s := &NetlinkSocket{
		fd: int32(fd),
	}
	s.lsa.Family = unix.AF_NETLINK
	if err := unix.Bind(fd, &s.lsa); err != nil {
		unix.Close(fd)
		return nil, err
	}

	return s, nil
}

// GetNetlinkSocketAt opens a netlink socket in the network namespace newNs
// and positions the thread back into the network namespace specified by curNs,
// when done. If curNs is close, the function derives the current namespace and
// moves back into it when done. If newNs is close, the socket will be opened
// in the current network namespace.
func GetNetlinkSocketAt(newNs, curNs netns.NsHandle, protocol int) (*NetlinkSocket, error) {
	c, err := executeInNetns(newNs, curNs)
	if err != nil {
		return nil, err
	}
	defer c()
	return getNetlinkSocket(protocol)
}

// executeInNetns sets execution of the code following this call to the
// network namespace newNs, then moves the thread back to curNs if open,
// otherwise to the current netns at the time the function was invoked
// In case of success, the caller is expected to execute the returned function
// at the end of the code that needs to be executed in the network namespace.
// Example:
//
//	func jobAt(...) error {
//	     d, err := executeInNetns(...)
//	     if err != nil { return err}
//	     defer d()
//	     < code which needs to be executed in specific netns>
//	 }
//
// TODO: his function probably belongs to netns pkg.
func executeInNetns(newNs, curNs netns.NsHandle) (func(), error) {
	var (
		err       error
		moveBack  func(netns.NsHandle) error
		closeNs   func() error
		unlockThd func()
	)
	restore := func() {
		// order matters
		if moveBack != nil {
			moveBack(curNs)
		}
		if closeNs != nil {
			closeNs()
		}
		if unlockThd != nil {
			unlockThd()
		}
	}
	if newNs.IsOpen() {
		runtime.LockOSThread()
		unlockThd = runtime.UnlockOSThread
		if !curNs.IsOpen() {
			if curNs, err = netns.Get(); err != nil {
				restore()
				return nil, fmt.Errorf("could not get current namespace while creating netlink socket: %v", err)
			}
			closeNs = curNs.Close
		}
		if err := netns.Set(newNs); err != nil {
			restore()
			return nil, fmt.Errorf("failed to set into network namespace %d while creating netlink socket: %v", newNs, err)
		}
		moveBack = netns.Set
	}
	return restore, nil
}

// Create a netlink socket with a given protocol (e.g. NETLINK_ROUTE)
// and subscribe it to multicast groups passed in variable argument list.
// Returns the netlink socket on which Receive() method can be called
// to retrieve the messages from the kernel.
func Subscribe(protocol int, groups ...uint) (*NetlinkSocket, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, protocol)
	if err != nil {
		return nil, err
	}
	s := &NetlinkSocket{
		fd: int32(fd),
	}
	s.lsa.Family = unix.AF_NETLINK

	for _, g := range groups {
		s.lsa.Groups |= (1 << (g - 1))
	}

	if err := unix.Bind(fd, &s.lsa); err != nil {
		unix.Close(fd)
		return nil, err
	}

	return s, nil
}

// SubscribeAt works like Subscribe plus let's the caller choose the network
// namespace in which the socket would be opened (newNs). Then control goes back
// to curNs if open, otherwise to the netns at the time this function was called.
func SubscribeAt(newNs, curNs netns.NsHandle, protocol int, groups ...uint) (*NetlinkSocket, error) {
	c, err := executeInNetns(newNs, curNs)
	if err != nil {
		return nil, err
	}
	defer c()
	return Subscribe(protocol, groups...)
}

func (s *NetlinkSocket) Close() {
	fd := int(atomic.SwapInt32(&s.fd, -1))
	unix.Close(fd)
}

func (s *NetlinkSocket) GetFd() int {
	return int(atomic.LoadInt32(&s.fd))
}

func (s *NetlinkSocket) Send(request *NetlinkRequest) error {
	fd := int(atomic.LoadInt32(&s.fd))
	if fd < 0 {
		return fmt.Errorf("Send called on a closed socket")
	}
	if err := unix.Sendto(fd, request.Serialize(), 0, &s.lsa); err != nil {
		return err
	}
	return nil
}

func (s *NetlinkSocket) Receive() ([]syscall.NetlinkMessage, *unix.SockaddrNetlink, error) {
	fd := int(atomic.LoadInt32(&s.fd))
	if fd < 0 {
		return nil, nil, fmt.Errorf("Receive called on a closed socket")
	}
	var fromAddr *unix.SockaddrNetlink
	var rb [RECEIVE_BUFFER_SIZE]byte
	nr, from, err := unix.Recvfrom(fd, rb[:], 0)
	if err != nil {
		return nil, nil, err
	}
	fromAddr, ok := from.(*unix.SockaddrNetlink)
	if !ok {
		return nil, nil, fmt.Errorf("Error converting to netlink sockaddr")
	}
	if nr < unix.NLMSG_HDRLEN {
		return nil, nil, fmt.Errorf("Got short response from netlink")
	}
	rb2 := make([]byte, nr)
	copy(rb2, rb[:nr])
	nl, err := syscall.ParseNetlinkMessage(rb2)
	if err != nil {
		return nil, nil, err
	}
	return nl, fromAddr, nil
}

// SetSendTimeout allows to set a send timeout on the socket
func (s *NetlinkSocket) SetSendTimeout(timeout *unix.Timeval) error {
	// Set a send timeout of SOCKET_SEND_TIMEOUT, this will allow the Send to periodically unblock and avoid that a routine
	// remains stuck on a send on a closed fd
	return unix.SetsockoptTimeval(int(s.fd), unix.SOL_SOCKET, unix.SO_SNDTIMEO, timeout)
}

// SetReceiveTimeout allows to set a receive timeout on the socket
func (s *NetlinkSocket) SetReceiveTimeout(timeout *unix.Timeval) error {
	// Set a read timeout of SOCKET_READ_TIMEOUT, this will allow the Read to periodically unblock and avoid that a routine
	// remains stuck on a recvmsg on a closed fd
	return unix.SetsockoptTimeval(int(s.fd), unix.SOL_SOCKET, unix.SO_RCVTIMEO, timeout)
}

// SetReceiveBufferSize allows to set a receive buffer size on the socket
func (s *NetlinkSocket) SetReceiveBufferSize(size int, force bool) error {
	opt := unix.SO_RCVBUF
	if force {
		opt = unix.SO_RCVBUFFORCE
	}
	return unix.SetsockoptInt(int(s.fd), unix.SOL_SOCKET, opt, size)
}

// SetExtAck requests error messages to be reported on the socket
func (s *NetlinkSocket) SetExtAck(enable bool) error {
	var enableN int
	if enable {
		enableN = 1
	}

	return unix.SetsockoptInt(int(s.fd), unix.SOL_NETLINK, unix.NETLINK_EXT_ACK, enableN)
}

func (s *NetlinkSocket) GetPid() (uint32, error) {
	fd := int(atomic.LoadInt32(&s.fd))
	lsa, err := unix.Getsockname(fd)
	if err != nil {
		return 0, err
	}
	switch v := lsa.(type) {
	case *unix.SockaddrNetlink:
		return v.Pid, nil
	}
	return 0, fmt.Errorf("Wrong socket type")
}

func ParseRouteAttr(b []byte) ([]syscall.NetlinkRouteAttr, error) {
	var attrs []syscall.NetlinkRouteAttr
	for len(b) >= unix.SizeofRtAttr {
		a, vbuf, alen, err := netlinkRouteAttrAndValue(b)
		if err != nil {
			return nil, err
		}
		ra := syscall.NetlinkRouteAttr{Attr: syscall.RtAttr(*a), Value: vbuf[:int(a.Len)-unix.SizeofRtAttr]}
		attrs = append(attrs, ra)
		b = b[alen:]
	}
	return attrs, nil
}

// ParseRouteAttrAsMap parses provided buffer that contains raw RtAttrs and returns a map of parsed
// atttributes indexed by attribute type or error if occured.
func ParseRouteAttrAsMap(b []byte) (map[uint16]syscall.NetlinkRouteAttr, error) {
	attrMap := make(map[uint16]syscall.NetlinkRouteAttr)

	attrs, err := ParseRouteAttr(b)
	if err != nil {
		return nil, err
	}

	for _, attr := range attrs {
		attrMap[attr.Attr.Type] = attr
	}
	return attrMap, nil
}

func netlinkRouteAttrAndValue(b []byte) (*unix.RtAttr, []byte, int, error) {
	a := (*unix.RtAttr)(unsafe.Pointer(&b[0]))
	if int(a.Len) < unix.SizeofRtAttr || int(a.Len) > len(b) {
		return nil, nil, 0, unix.EINVAL
	}
	return a, b[unix.SizeofRtAttr:], rtaAlignOf(int(a.Len)), nil
}

// SocketHandle contains the netlink socket and the associated
// sequence counter for a specific netlink family
type SocketHandle struct {
	Seq    uint32
	Socket *NetlinkSocket
}

// Close closes the netlink socket
func (sh *SocketHandle) Close() {
	if sh.Socket != nil {
		sh.Socket.Close()
	}
}
