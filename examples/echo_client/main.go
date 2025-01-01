package main

import (
	"context"
	"fmt"
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
	ctx := context.Background()
	_, err = client.Connect(ctx, &unix.SockaddrInet4{Port: config.Port, Addr: config.Host})
	if err != nil {
		log.Fatal(err)
	}
	hostPort := fmt.Sprintf("%s:%d", netip.AddrFrom4(config.Host), config.Port)
	ws, err := wsoding.Connect(ctx, client, hostPort, "/")
	if err != nil {
		log.Fatal(err)
	}
	echo.Serve(ws)

}
