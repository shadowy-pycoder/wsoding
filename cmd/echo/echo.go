package echo

import (
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
		fmt.Println(message)

		if err != nil {
			log.Fatal(err)
			// TODO: Tuck sending the CLOSE frame under some abstraction of "Closing the WebSocket".
			// Maybe some sort of ws.close() method.
			// TODO: The sender may give a reason of the close via the status code
			// See RFC6466, Section 7.4
			err = ws.SendFrame(true, wsoding.WSOpcode(0x8), []byte{})
			if err != nil {
				log.Fatal(err)
			}
			break
		}
		ws.SendMessage(message.Kind, message.Payload)
		fmt.Printf("INFO: %d: %s sent: %d bytes", i, peerWho, len(message.Payload))
	}
}
