package main

import (
	"context"
	"log"
	"net/netip"
	"syscall"

	"github.com/mdlayher/socket"
	"github.com/shadowy-pycoder/wsoding"
	"github.com/shadowy-pycoder/wsoding/cmd/echo"
	"golang.org/x/sys/unix"
)

var (
	host = [4]byte{0x7f, 0x000, 0x00, 0x01}
	port = 9001
)

func main() {
	client, err := socket.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0, "wsoding-client", nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Connect(context.TODO(), &unix.SockaddrInet4{Port: port, Addr: host})
	if err != nil {
		log.Fatal(err)
	}
	ws, err := wsoding.Connect(client, netip.AddrFrom4(host).String(), "/")
	if err != nil {
		log.Fatal(err)
	}
	go echo.Serve(ws)

}
