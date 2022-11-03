package main

import (
	"net/http"

	gecgosio "github.com/lulzsun/gecgos.io"
)

func main() {
	fileServer := http.FileServer(http.Dir("./public"))
	http.FileServer(http.Dir("./public"))
	http.Handle("/", fileServer)

	server := gecgosio.Server{
		Ordered: true,
	}
	server.Listen(420)
}
