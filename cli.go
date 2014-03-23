// +build ignore

// Command line tool for checking minecraft server status

package main

import "flag"
import "fmt"
import "time"
import "regexp"
import "strconv"
import "io/ioutil"

import "github.com/Craftserve/mcstatus"

var COLOR_EXPR = regexp.MustCompile("(?i)§[a-z]")

var serialize = flag.Bool("serialize", false, "serialize output as json")
var saveicon = flag.Bool("saveicon", false, "save icon to icon.png")

func main() {
	flag.Parse()
	if *serialize {
		for _, v := range flag.Args() {
			addr, err := mcstatus.Resolve(v)
			if err != nil {
				fmt.Printf("{name:%s, error:'error resolving: %s\n'}", v, err)
				continue
			}
			// (status *MinecraftStatus, ping time.Duration, err error)
			status, _, err := mcstatus.CheckStatus(addr)
			if err != nil {
				fmt.Printf("{name:%s, error:'error checking: %s\n'}", v, err)
				continue
			}
			s, _ := status.SerializeNew()
			fmt.Printf(string(s) + "\n")
		}
		return
	}

	for _, v := range flag.Args() {
		addr, err := mcstatus.Resolve(v)
		if err != nil {
			fmt.Printf("%s: error resolving: %s\n", v, err)
			continue
		}
		// (status *MinecraftStatus, ping time.Duration, err error)
		status, ping, err := mcstatus.CheckStatus(addr)
		if err != nil {
			fmt.Printf("%s: error checking: %s\n", v, err)
			continue
		}
		checkproto := "old"
		if status.NewProtocol {
			checkproto = "new"
		}
		descr := COLOR_EXPR.ReplaceAllString(status.Description, "")
		fmt.Printf("%-30s %-4d / %-5d (%-5s %-5q %s) \"%s\"\n", v, status.Players, status.Slots, strconv.Itoa(int(ping/time.Millisecond))+"ms", status.GameVersion, checkproto, descr)
		if *saveicon && status.Favicon != nil {
			ioutil.WriteFile("icon.png", status.Favicon, 0644)
			*saveicon = false
		}
	}
}
