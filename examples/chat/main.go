package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"maps"
	"net/netip"
	"sync"
	"syscall"

	"github.com/mdlayher/socket"
	"github.com/shadowy-pycoder/wsoding"
	"github.com/shadowy-pycoder/wsoding/examples/internal/config"
	"golang.org/x/sys/unix"
)

type Clients struct {
	sync.Mutex
	conn map[string]wsoding.WS
}

func main() {
	clients := Clients{conn: make(map[string]wsoding.WS)}
	server, err := socket.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0, "wsoding-chat", nil)
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
	ctx := context.Background()
	for {
		client, addr, err := server.Accept(ctx, 0)
		if err != nil {
			log.Println(err)
			continue
		}
		address := (addr).(*unix.SockaddrInet4)
		addrStr := fmt.Sprintf("%s:%d", netip.AddrFrom4(address.Addr), address.Port)
		fmt.Printf("%s Client connected\n", addrStr)
		ws, err := wsoding.Accept(ctx, client)
		if err != nil {
			log.Println(err)
			if err = client.Close(); err != nil {
				log.Println(err)
				continue
			}
		}
		ws.Debug = true
		clients.Lock()
		clients.conn[addrStr] = ws
		clients.Unlock()
		for c := range maps.Values(clients.conn) {
			err = c.SendMessage(wsoding.MessageTEXT, []byte(fmt.Sprintf("%s Joined the chat", addrStr)))
			if err != nil {
				log.Println(err)
			}
		}
		go (func() {
			defer (func() {
				if err := ws.SendFrame(true, wsoding.OpCodeCLOSE, []byte{}); err != nil {
					log.Println(err)
				}
				if err := ws.Close(); err != nil {
					log.Println(err)
				}
				clients.Lock()
				delete(clients.conn, addrStr)
				clients.Unlock()
			})()
			for {
				message, err := ws.ReadMessage()
				if err != nil {
					log.Println(err)
					break
				}
				for c := range maps.Values(clients.conn) {
					err = c.SendMessage(message.Kind, bytes.Join([][]byte{[]byte(addrStr), message.Payload}, []byte(" ")))
					if err != nil {
						log.Println(err)
						break
					}
				}
			}
		})()
	}
}
