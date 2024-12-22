package main

import (
	"context"
	"log"
	"net/netip"
	"syscall"

	"github.com/mdlayher/socket"
	"github.com/shadowy-pycoder/wsoding"
	"github.com/shadowy-pycoder/wsoding/examples/internal/config"
	"github.com/shadowy-pycoder/wsoding/examples/internal/echo"
	"golang.org/x/sys/unix"
)

func main() {
	client, err := socket.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0, "wsoding-client", nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Connect(context.TODO(), &unix.SockaddrInet4{Port: config.Port, Addr: config.Host})
	if err != nil {
		log.Fatal(err)
	}
	ws, err := wsoding.Connect(client, netip.AddrFrom4(config.Host).String(), "/")
	if err != nil {
		log.Fatal(err)
	}
	go echo.Serve(ws)

}
