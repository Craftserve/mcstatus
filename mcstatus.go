package mcstatus

import "net"
import "fmt"
import "time"
import "strings"
import "strconv"
import "encoding/json"
import "regexp"

type MinecraftStatus struct {
	NewProtocol     bool
	ProtocolVersion int
	GameVersion     string
	Slots           int
	Players         int
	PlayersSample   []string
	Description     string
	Favicon         []byte
	Raw             *json.RawMessage `json:"-"`
}

var LAST_NEW_PROTOCOL int = 4
var LAST_OLD_PROTOCOL uint8 = 74
var DEADLINE = 3 * time.Second
var COLOR_EXPR = regexp.MustCompile("(?i)§[a-z]")

// check minecraft status, first tries new protocol, later older
func CheckStatus(addr *net.TCPAddr) (status *MinecraftStatus, ping time.Duration, err error) {
	status, ping, err = CheckStatusNew(addr, addr.IP.String(), uint16(addr.Port))
	if err == nil {
		return
	}
	status, ping, err = CheckStatusOld(addr, addr.IP.String(), uint16(addr.Port))
	return
}

// resolves addres minecraft style, with dumb SRV records
func Resolve(hostport string) (addr *net.TCPAddr, err error) {
	// direct resolving with known port
	if strings.Contains(hostport, ":") {
		addr, err = net.ResolveTCPAddr("tcp4", hostport)
		return
	}
	// DNS SRV records
	_, addrs, err := net.LookupSRV("minecraft", "tcp", hostport)
	if err == nil {
		addr, err = net.ResolveTCPAddr("tcp4", addrs[0].Target+":"+strconv.Itoa(int(addrs[0].Port)))
		return
	}
	// Fallback to A record with default port
	addr, err = net.ResolveTCPAddr("tcp4", hostport+":25565")
	return
}

func (status *MinecraftStatus) String() string {
	descr := COLOR_EXPR.ReplaceAllString(status.Description, "")
	if len(descr) > 20 {
		descr = descr[:20]
	}
	return fmt.Sprintf("MinecraftStatus(%d / %d (%s) \"%s\")", status.Players, status.Slots, status.GameVersion, descr)
}
