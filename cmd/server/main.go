package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
)

type Broker struct {
	sockets map[net.Conn]struct{}
	sync.Mutex
}

var broker Broker

func main() {
	port := flag.Int("port", 8000, "port to listen on")
	flag.Parse()

	broker.sockets = make(map[net.Conn]struct{})

	portString := fmt.Sprintf(":%d", *port)
	tcpListener, err := net.Listen("tcp", portString)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := tcpListener.Accept()
		if err != nil {
			log.Fatal(err)

		}

		broker.Lock()
		broker.sockets[conn] = struct{}{}
		broker.Unlock()
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	readFailCount := 0
	for {
		if readFailCount > 8 {
			break
		}

		message, err := reader.ReadBytes('\n')
		if err != nil {
			log.Print("failed to read from connection:", err)
			readFailCount++

			continue
		}

		broker.Lock()
		for client := range broker.sockets {
			if client == conn {
				continue
			}
			if _, err := client.Write(message); err != nil {
				log.Print("failed to relay event to connection:", err)
			}
		}
		broker.Unlock()
	}

	broker.Lock()
	delete(broker.sockets, conn)
	broker.Unlock()
}
