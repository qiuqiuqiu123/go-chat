package main

import "testProject/server"

func main() {
	wsServer := server.NewServer("wsServer", ":9095")
	wsServer.Start()
}
