package mcstatus

import "net"
import "time"
import "encoding/binary"
import "unicode/utf16"
import "bytes"
import "strconv"
import "fmt"
import "strings"
import "io"
import "reflect"
import "unsafe"

func CheckStatusOld(addr *net.TCPAddr, host string, port uint16) (*MinecraftStatus, time.Duration, error) {
	conn, err := net.DialTCP(addr.Network(), nil, addr)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusOld error dialing: %s", err)
	}
	defer conn.Close()
	status, ping, err := CheckStatusOldConn(conn, host, port)
	return status, ping, err
}

func CheckStatusOldConn(conn *net.TCPConn, host string, port uint16) (*MinecraftStatus, time.Duration, error) {
	buff := &bytes.Buffer{}
	buff.Grow(512)
	buff.Write([]byte{0xFE, 0x01, 0xFA})
	buff.Write(pack_utf16be("MC|PingHost"))
	hostpacked := pack_utf16be(host)
	binary.Write(buff, binary.BigEndian, int16(len(hostpacked)+5)) // host + uint8 protocol + int32 port
	binary.Write(buff, binary.BigEndian, uint8(LAST_OLD_PROTOCOL))
	buff.Write(hostpacked)
	binary.Write(buff, binary.BigEndian, int32(port))

	conn.SetDeadline(time.Now().Add(DEADLINE))
	_, err := conn.Write(buff.Bytes())
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusOld error sending: %s", err)
	}

	t1 := time.Now()

	var c [1]byte
	_, err = conn.Read(c[:])
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusOld error reading packet id: %s", err)
	}
	if c[0] != 0xFF {
		return nil, 0, fmt.Errorf("CheckStatusOld bad response packet id: %d", c[0])
	}
	msg, err := read_utf16be(conn, 512)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusOld error reading response string: %s", err)
	}

	// TODO: zastanowic sie czy nie lepiej uzyc fmt.Sscanf
	status := &MinecraftStatus{}
	if strings.HasPrefix(msg, "§1") { // 1.4 +
		params := strings.Split(msg, "\x00")
		if len(params) != 6 {
			return nil, 0, fmt.Errorf("CheckStatusOld bad param count %d (1)", len(params))
		}
		status.ProtocolVersion, err = strconv.Atoi(params[1])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusOld error converting protocol version")
		}
		status.GameVersion = params[2]
		status.Description = params[3]
		status.Players, err = strconv.Atoi(params[4])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusOld error converting player count")
		}
		status.Slots, err = strconv.Atoi(params[5])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusOld error converting slot count")
		}

	} else { // < 1.4
		params := strings.Split(msg, "§")
		if len(params) != 3 {
			return nil, 0, fmt.Errorf("CheckStatusOld bad param count %d (2)", len(params))
		}
		// last protocol and game version with this response format
		status.ProtocolVersion = 39
		status.GameVersion = "1.3.1"
		status.Description = params[0]
		status.Players, err = strconv.Atoi(params[1])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusOld error converting player count")
		}
		status.Slots, err = strconv.Atoi(params[2])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusOld error converting slot count")
		}
	}

	return status, time.Since(t1), nil
}

func pack_utf16be(s string) []byte {
	var shorts []uint16
	shorts = utf16.Encode([]rune(s))

	buff := make([]byte, 2+(len(shorts)*2))
	length := int16(len(shorts))
	binary.BigEndian.PutUint16(buff, uint16(length))

	header := *(*reflect.SliceHeader)(unsafe.Pointer(&shorts))
	header.Len = len(shorts) * 2
	bytes := *(*[]byte)(unsafe.Pointer(&header))
	for i := 0; i < len(bytes); i += 2 {
		shorts[i/2] = (uint16(bytes[i]) << 8) | uint16(bytes[i+1])
	}

	copy(buff[2:], bytes)
	return buff
}

func read_utf16be(reader io.Reader, max_length int) (string, error) {
	var length int16
	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		return "", err
	}
	if int(length) > max_length {
		return "", fmt.Errorf("String longer than max_length: %d > %d", length, max_length)
	}
	if length < 0 {
		return "", fmt.Errorf("String length smaller than 0: %d", length)
	}

	utf16be := make([]byte, length*2)
	n, err := io.ReadFull(reader, utf16be)
	if err != nil {
		return "", err
	}
	if n < int(length*2) {
		return "", fmt.Errorf("Reader returned less data than string length!")
	}

	// > BASED ON http://play.golang.org/p/xtG1e9iqA1
	// Convert to []uint16
	// hop through unsafe to skip any actual allocation/copying
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&utf16be))
	header.Len = len(utf16be) / 2
	shorts := *(*[]uint16)(unsafe.Pointer(&header))
	// shorts may need byte-swapping
	for i := 0; i < n; i += 2 {
		shorts[i/2] = (uint16(utf16be[i]) << 8) | uint16(utf16be[i+1])
	}

	runes := utf16.Decode(shorts)
	return string(runes), nil
}

func (status *MinecraftStatus) SerializeOld(protocol int) ([]byte, error) {
	var s string
	if protocol > 39 { // 1.4 +
		s = fmt.Sprintf("§1\x00%d\x00%s\x00%s\x00%d\x00%d", status.ProtocolVersion, status.GameVersion, status.Description, status.Players, status.Slots)
	} else {
		descr := COLOR_EXPR.ReplaceAllString(status.Description, "")
		s = fmt.Sprintf("%s§%d§%d", descr, status.Players, status.Slots)
	}
	return []byte(s), nil
}
