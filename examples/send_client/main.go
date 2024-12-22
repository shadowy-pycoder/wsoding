package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/netip"
	"os"
	"strconv"
	"syscall"

	"github.com/mdlayher/socket"
	"github.com/shadowy-pycoder/wsoding"
	"github.com/shadowy-pycoder/wsoding/examples/internal/config"
	"golang.org/x/sys/unix"
)

func shift(xs *[]string) string {
	var x = (*xs)[0]
	*xs = (*xs)[1:]
	return x
}

func usage(programName string) {
	fmt.Printf("Usage: %s <host> <port> <message>", programName)
}

func main() {
	args := os.Args
	programName := shift(&args)
	if len(args) == 0 {
		usage(programName)
		log.Fatal("ERROR: no host is provided")
	}
	host, err := netip.ParseAddr(shift(&args))
	if err != nil {
		log.Fatalf("ERROR: host is invalid: %s\n", err)
	}
	if len(args) == 0 {
		usage(programName)
		log.Fatal("ERROR: no port is provided")
	}
	port, err := strconv.Atoi(shift(&args))
	if err != nil {
		log.Fatalf("ERROR: port is not a valid integer: %s\n", err)
	}
	if len(args) == 0 {
		usage(programName)
		log.Fatal("ERROR: no message is provided")
	}
	message := shift(&args)
	client, err := socket.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0, "wsoding-send-client", nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Connect(context.TODO(), &unix.SockaddrInet4{Port: port, Addr: host.As4()})
	if err != nil {
		log.Fatal(err)
	}
	ws := wsoding.WS{
		Sock:   client,
		Debug:  true,
		Client: true,
	}
	defer (func() {
		if err := ws.SendFrame(true, wsoding.OpCodeCLOSE, []byte{}); err != nil {
			log.Fatal(err)
		}
		if err = ws.Close(); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Fatal(err)
			}
		}
	})()
	hostPort := fmt.Sprintf("%s:%d", host, config.Port)
	if err := ws.ClientHandshake(hostPort, "/"); err != nil {
		log.Fatal(err)
	}
	if err := ws.SendText(message); err != nil {
		log.Fatal(err)
	}
	messageFromServer, err := ws.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Message from server: %s len: %d\n", messageFromServer.Payload, len(messageFromServer.Payload))
}
