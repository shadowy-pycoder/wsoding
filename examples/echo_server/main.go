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
	// TODO: Turn example_server into an asynchronous echo server that just continuosly echos all the client messages
	// until the client closes the connection. I think some of the Autobahn Test Cases depends on this exact behavior.
	// This may require implementing proper periodic pinging of the clients and closing those who fell off.
	// (Which I believe is also part of some of the Autobahn Test Cases).
	server, err := socket.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0, "wsoding-server", nil)
	if err != nil {
		log.Fatal(err)
	}
	err = server.SetsockoptInt(syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		log.Fatal(err)
	}
	err = server.Bind(&unix.SockaddrInet4{Port: config.Port, Addr: config.Host})
	if err != nil {
		log.Fatal(err)
	}
	err = server.Listen(10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Listening to %s:%d\n", netip.AddrFrom4(config.Host), config.Port)
	for {
		client, addr, err := server.Accept(context.TODO(), 0)
		if err != nil {
			log.Println(err)
			continue
		}
		address := (addr).(*unix.SockaddrInet4)
		fmt.Printf("%s:%d Client connected\n", netip.AddrFrom4(address.Addr), address.Port)
		ws, err := wsoding.Accept(client)
		if err != nil {
			log.Println(err)
			if err = client.Close(); err != nil {
				log.Println(err)
			}
		}
		ws.Debug = true
		go echo.Serve(ws)
	}
}
