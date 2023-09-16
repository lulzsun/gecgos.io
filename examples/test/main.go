package main

import (
	"fmt"
	"net/http"

	gecgosio "github.com/lulzsun/gecgos.io"
)

func main() {
	fileServer := http.FileServer(http.Dir("./public"))
	http.FileServer(http.Dir("./public"))
	http.Handle("/", fileServer)

	server := gecgosio.Gecgos(nil)

	server.OnConnection(func(peer gecgosio.Peer) {
		fmt.Printf("Client %s has connected!\n", peer.Id)

		// Example of sending and recieving from client(s)
		// Server will recieve the event 'ping' with data 'hello'
		// Server will send the event 'pong' with data 'world'
		peer.On("ping", func(msg string) {
			fmt.Printf("Client %s sent event 'ping' with data '%s', emitting back 'pong'\n", peer.Id, msg)
			// peer.Reliable(150, 10).Emit("pong", "world")
			peer.Emit("pong", "world")
		})
	})

	server.OnDisconnect(func(peer gecgosio.Peer) {
		fmt.Printf("Client %s has disconnected!\n", peer.Id)
	})

	server.Listen(420)
}
