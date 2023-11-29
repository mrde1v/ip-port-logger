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

	go startServer("tcp")
	go startServer("udp")

	startWorkerPool()

	go monitorPackets()

	select {}
}

func startServer(network string) {
	var listener net.Listener
	var err error

	if network == "tcp" {
		listener, err = net.Listen(network, listenAddress)
		if err != nil {
			fmt.Printf("Error starting %s server: %s\n", network, err)
			os.Exit(1)
		}
		defer listener.Close()

		fmt.Printf("%s Server listening on %s\n", network, listenAddress)

		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Printf("Error accepting %s connection: %s\n", network, err)
				continue
			}

			go handleConnection(conn)
		}
	} else if network == "udp" {
		udpAddr, err := net.ResolveUDPAddr(network, listenAddress)
		if err != nil {
			fmt.Printf("Error resolving UDP address: %s\n", err)
			return
		}
		conn, err := net.ListenUDP(network, udpAddr)
		if err != nil {
			fmt.Printf("Error starting %s server: %s\n", network, err)
			os.Exit(1)
		}
		defer conn.Close()

		fmt.Printf("%s Server listening on %s\n", network, listenAddress)

		for {
			handleUDPConnection(conn)
		}
	} else {
		fmt.Printf("Unsupported network type: %s\n", network)
		os.Exit(1)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	// fmt.Printf("Connection from %s\n", remoteAddr)

	reader := bufio.NewReader(conn)

	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				processData(remoteAddr)
				// fmt.Printf("Connection from %s closed by the client\n", remoteAddr)
				return
			}
			fmt.Println("Error reading from connection:", err)
			return
		}

		go processData(remoteAddr)
	}
}

func handleUDPConnection(conn *net.UDPConn) {
	buffer := make([]byte, 1024)
	_, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Error reading from UDP Connection: %s\n", err)
		return
	}

	ipport := fmt.Sprint(addr)
	processData(ipport)
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

	// fmt.Println(packetCounts[remoteAddr])

	if packetCounts[remoteAddr] > packetLimit {
		go logIP(remoteAddr)
		packetCounts[remoteAddr] = 0
	}
}

func logIP(ip string) {
	ipport := strings.Split(ip, ":")
	if ipExistsInFile(ipport[0]) {
		return
	}

	// fmt.Println("logging")

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
