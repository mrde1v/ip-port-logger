package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	listenPort  = "0.0.0.0:6969"
	packetLimit = 3
	logFileName = "ips.txt"
)

var (
	packetCounts map[string]int
	packetMutex  sync.Mutex
)

func main() {
	packetCounts = make(map[string]int)

	go startServer()

	go monitorPackets()

	select {}
}

func startServer() {
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		fmt.Println("Error starting server: ", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s\n", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	fmt.Printf("Connection from %s\n", remoteAddr)

	for {
		data, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				processData(remoteAddr, data)
				return
			}
			fmt.Println("Error reading from connection: ", err)
			return
		}

		processData(remoteAddr, data)
	}
}

func processData(remoteAddr, data string) {
	packetMutex.Lock()
	packetCounts[remoteAddr]++
	packetMutex.Unlock()

	fmt.Printf("Received data from %s: %s\n", remoteAddr, data)
}

func monitorPackets() {
	for {
		time.Sleep(time.Second)

		packetMutex.Lock()
		for ip, count := range packetCounts {
			if count > packetLimit {
				logIP(ip)
			}
			packetCounts[ip] = 0
		}
		packetMutex.Unlock()
	}
}

func logIP(ip string) {
	if ipExistsInFile(ip) {
		return
	}

	file, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error opening log file: ", err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(ip + "\n"); err != nil {
		fmt.Println("Error writing to log file: ", err)
	}
	fmt.Printf("IP %s exceeded packet limit\n", ip)
}

func ipExistsInFile(ip string) bool {
	file, err := os.Open(logFileName)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == ip {
			return true
		}
	}

	return false
}
