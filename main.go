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
	listenAddress = "0.0.0.0:6969"
	packetLimit   = 1
	logFileName   = "ips.txt"
	numWorkers    = 10
)

var (
	packetCounts map[string]int
	packetMutex  sync.Mutex
	logMutex     sync.Mutex
)

func main() {
	packetCounts = make(map[string]int)

	go startServer()

	startWorkerPool()

	go monitorPackets()

	select {}
}

func startServer() {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s\n", listenAddress)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	fmt.Printf("Connection from %s\n", remoteAddr)

	reader := bufio.NewReader(conn)

	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				processData(remoteAddr)
				fmt.Printf("Connection from %s closed by the client\n", remoteAddr)
				return
			}
			fmt.Println("Error reading from connection:", err)
			return
		}

		go processData(remoteAddr)
	}
}

func startWorkerPool() {
	for i := 0; i < numWorkers; i++ {
		go worker()
	}
}

func worker() {
	for {
		select {
		case job := <-jobQueue:
			processData(job.remoteAddr)
		}
	}
}

type job struct {
	remoteAddr string
}

var jobQueue = make(chan job, 100)

func processData(remoteAddr string) {
	packetMutex.Lock()
	packetCounts[remoteAddr]++
	packetMutex.Unlock()

	fmt.Println(packetCounts[remoteAddr])

	if packetCounts[remoteAddr] > packetLimit {
		logIP(remoteAddr)
		packetCounts[remoteAddr] = 0
	}
}

func logIP(ip string) {
	ipport := strings.Split(ip, ":")
	if ipExistsInFile(ipport[0]) {
		return
	}

	fmt.Println("logging")

	logMutex.Lock()
	defer logMutex.Unlock()

	file, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(ipport[0] + "\n"); err != nil {
		fmt.Println("Error writing to log file:", err)
	}
	fmt.Printf("IP %s exceeded packet limit and is logged in %s\n", ip, logFileName)
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
