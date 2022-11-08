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

	server := gecgosio.Server{
		Ordered: true,
		Cors:    gecgosio.Cors{Origin: "*"},
	}

	server.OnConnect(func(c gecgosio.Client) {
		fmt.Printf("Client %s has connected!\n", c.Id)
	})

	server.OnDisconnect(func(c gecgosio.Client) {
		fmt.Printf("Client %s has disconnected!\n", c.Id)
	})

	// Example of sending and recieving from client(s)
	// Server will recieve the event 'ping' with data 'hello'
	// Server will send the event 'pong' with data 'world'
	server.On("ping", func(c gecgosio.Client, msg string) {
		fmt.Printf("Client %s sent event 'ping' with data '%s', emitting back 'pong'\n", c.Id, msg)
		c.Emit("pong", "world")
	})

	server.Listen(420)
}
