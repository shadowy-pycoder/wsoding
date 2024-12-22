package echo

import (
	"errors"
	"fmt"
	"log"

	"github.com/shadowy-pycoder/wsoding"
)

func Serve(ws wsoding.WS) {
	defer ws.Close()
	peerWho := "Server"
	if ws.Client {
		peerWho = "Client"
	}
	for i := 0; ; i++ {
		message, err := ws.ReadMessage()
		if err != nil {
			if errors.Is(err, wsoding.ErrCloseFrameSent) {
				log.Printf("INFO: %s closed connection\n", peerWho)
			} else {
				log.Printf("ERROR: %s connection failed: %s\n", peerWho, err)
			}
			// TODO: Tuck sending the CLOSE frame under some abstraction of "Closing the WebSocket".
			// Maybe some sort of ws.close() method.
			// TODO: The sender may give a reason of the close via the status code
			// See RFC6466, Section 7.4
			err = ws.SendFrame(true, wsoding.OpCodeCLOSE, []byte{})
			if err != nil {
				log.Println(err)
			}
			break
		}
		err = ws.SendMessage(message.Kind, message.Payload)
		if err != nil {
			log.Println(err)
			break
		}
		fmt.Printf("INFO: %d: %s sent: %d bytes\n", i, peerWho, len(message.Payload))
	}
}
