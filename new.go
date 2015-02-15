package mcstatus

import "net"
import "bytes"
import "encoding/binary"
import "encoding/json"
import "encoding/base64"
import "time"
import "io"
import "fmt"
import "strings"

func CheckStatusNew(addr *net.TCPAddr, host string, port uint16) (*MinecraftStatus, time.Duration, error) {
	conn, err := net.DialTCP(addr.Network(), nil, addr)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error dialing: %s", err)
	}
	defer conn.Close()
	status, ping, err := CheckStatusNewConn(conn, host, port)
	return status, ping, err
}

func CheckStatusNewConn(conn *net.TCPConn, host string, port uint16) (*MinecraftStatus, time.Duration, error) {
	conn.SetDeadline(time.Now().Add(DEADLINE))
	defer conn.SetDeadline(time.Time{})
	conn.SetNoDelay(true)

	buff := &bytes.Buffer{}
	buff.Write(pack_varint(LAST_NEW_PROTOCOL))
	buff.Write(pack_utf8(host))
	binary.Write(buff, binary.BigEndian, port)
	buff.Write(pack_varint(1)) // next_state = status
	err := write_packet(0x00, buff.Bytes(), conn)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error sending handshake: %s", err)
	}

	err = write_packet(0x00, []byte{}, conn)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error sending status req: %s", err)
	}

	t1 := time.Now()

	packet_id, payload, err := read_packet(conn)

	ping := time.Since(t1)

	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error reading status packet: %s", err)
	}
	if packet_id != 0x00 {
		return nil, 0, fmt.Errorf("CheckStatusNew invalid status packet id: %d", packet_id)
	}
	status_data, err := read_utf8(bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error reading status data: %s", err)
	}
	var status_new minecraftStatusNew
	err = json.Unmarshal([]byte(status_data), &status_new)
	if err != nil {
		return nil, 0, fmt.Errorf("CheckStatusNew error parsing status data: %s", err)
	}

	status := &MinecraftStatus{}
	raw := json.RawMessage(status_data)
	status.Raw = &raw
	status.NewProtocol = true
	status.ProtocolVersion = status_new.Version.Protocol
	status.GameVersion = status_new.Version.Name
	status.Slots = status_new.Players.Max
	status.Players = status_new.Players.Online
	status.PlayersSample = make([]string, 0, len(status_new.Players.Sample))
	for _, v := range status_new.Players.Sample {
		status.PlayersSample = append(status.PlayersSample, v.Name)
	}

	if str, ok := status_new.Description.(string); ok {
		status.Description = str
	} else if m, ok := status_new.Description.(map[string]interface{}); ok && m["text"] != nil {
		status.Description, ok = m["text"].(string)
		if !ok {
			return nil, 0, fmt.Errorf("CheckStatusNew invalid description format (2)")
		}
	} else {
		return nil, 0, fmt.Errorf("CheckStatusNew invalid description format (1)")
	}

	if strings.HasPrefix(status_new.Favicon, "data:image/png;base64,") {
		status.Favicon, err = base64.StdEncoding.DecodeString(status_new.Favicon[22:])
		if err != nil {
			return nil, 0, fmt.Errorf("CheckStatusNew invalid favicon base64")
		}
		//return nil, 0, fmt.Errorf("CheckStatusNew invalid favicon format: %v", status_new.Favicon)
	}

	return status, ping, nil
}

func (status *MinecraftStatus) SerializeNew() ([]byte, error) {
	if status.Raw != nil {
		return status.Raw.MarshalJSON()
	}

	d := &minecraftStatusNew{}
	if status.NewProtocol {
		d.Version.Protocol = status.ProtocolVersion
	}
	d.Version.Name = status.GameVersion
	d.Players.Max = status.Slots
	d.Players.Online = status.Players
	for _, v := range status.PlayersSample {
		r := minecraftStatusNewPlayer{Name: v, Id: "d0223ac4-a35d-43dc-96de-5fecdb8feecd"} // completely random uuid
		d.Players.Sample = append(d.Players.Sample, r)
	}
	d.Description = status.Description
	if len(status.Favicon) > 0 {
		d.Favicon = "data:image/png;base64," + base64.StdEncoding.EncodeToString(status.Favicon)
	}

	e, err := json.Marshal(d)
	return e, err
}

/*
{   "version": {
        "name": "13w41a",
        "protocol": 0
    },
    "players": {
        "max": 100,
        "online": 5,
        "sample":[
            {"name":"Thinkofdeath", "id":""}
        ]
    },
    "description": {"text":"Hello world"},
    "favicon": "data:image/png;base64,<data>"
} */

type minecraftStatusNewPlayer struct {
	Name string `json:"name"`
	Id   string `json:"id"`
}

type minecraftStatusNew struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int                        `json:"max"`
		Online int                        `json:"online"`
		Sample []minecraftStatusNewPlayer `json:"sample,omitempty"`
	} `json:"players"`
	Description interface{} `json:"description"`
	Favicon     string      `json:"favicon,omitempty"`
}

func pack_varint(c int) []byte {
	var buf [8]byte
	n := binary.PutUvarint(buf[:], uint64(uint32(c)))
	return buf[:n]
}

func read_varint(reader io.Reader) (int, error) {
	br, ok := reader.(io.ByteReader)
	if !ok {
		br = dummyByteReader{reader, [1]byte{}}
	}
	x, err := binary.ReadUvarint(br)
	return int(int32(uint32(x))), err
}

type dummyByteReader struct {
	io.Reader
	buf [1]byte
}

func (b dummyByteReader) ReadByte() (byte, error) {
	_, err := b.Read(b.buf[:])
	return b.buf[0], err
}

func write_packet(id int, payload []byte, writer io.Writer) error {
	id_enc := pack_varint(id)
	l := len(id_enc) + len(payload)
	l_enc := pack_varint(l)
	d := make([]byte, len(l_enc)+len(id_enc)+len(payload))
	n := copy(d[0:], l_enc)
	m := copy(d[n:], id_enc)
	k := copy(d[n+m:], payload)
	if k < len(payload) {
		panic("k < len(payload)")
	}
	_, err := writer.Write(d[0:])
	return err
}

func read_packet(reader io.Reader) (id int, payload []byte, err error) {
	l, err := read_varint(reader) // dlugosc zawiera tez id pakietu
	if err != nil {
		return
	}
	if l > 32768 || l < 0 {
		err = fmt.Errorf("read_packet: bad length %d", l)
		return
	}
	lr := &io.LimitedReader{reader, 10} // hack zeby wiedziec ile varint zajmowal
	id, err = read_varint(lr)
	if err != nil {
		return
	}
	payload_len := l - (10 - int(lr.N))
	if payload_len < 0 {
		err = fmt.Errorf("read_packet: bad payload length %d (full packet %d)", payload_len, l)
		return
	}
	payload = make([]byte, payload_len) // read rest of packet
	_, err = io.ReadFull(reader, payload)
	return
}

func pack_utf8(s string) []byte {
	data := pack_varint(len(s))
	data = append(data, []byte(s)...)
	return data
}

func read_utf8(reader io.Reader) (s string, err error) {
	length, err := read_varint(reader)
	if err != nil {
		return
	}
	d := make([]byte, length)
	_, err = reader.Read(d)
	if err != nil {
		return
	}
	return string(d), nil
}
