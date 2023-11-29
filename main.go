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
	listenAddress         = "0.0.0.0:6969"
	packetLimit           = 1
	logFileName           = "ips.txt"
	logSourcePortFileName = "srcports.txt"
	numWorkers            = 1000
	logInterval           = 10 * time.Second
)

var (
	packetCounts map[string]int
	packetMutex  sync.Mutex
	logMutex     sync.Mutex
	uniqueIPs    = make(map[string]struct{})
	uniquePorts  = make(map[string]struct{})
	uniqueMutex  sync.Mutex
	saveLogsCh   = make(chan struct{}, 1)
	shutdownCh   = make(chan struct{})
)

func main() {
	packetCounts = make(map[string]int)

	go startServer("tcp")
	go startServer("udp")

	startWorkerPool()

	go monitorPackets()

	go func() {
		for {
			select {
			case <-time.After(logInterval):
				saveLogsCh <- struct{}{}
			case <-shutdownCh:
				return
			}
		}
	}()

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
	reader := bufio.NewReader(conn)

	for {
		_, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				processData(remoteAddr)
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

	if packetCounts[remoteAddr] > packetLimit {
		go logIP(remoteAddr)
		go logPort(remoteAddr)
		packetCounts[remoteAddr] = 0
	}
}

func logIP(ip string) {
	uniqueMutex.Lock()
	defer uniqueMutex.Unlock()
	ipport := strings.Split(ip, ":")

	if _, exists := uniqueIPs[ipport[0]]; !exists {
		uniqueIPs[ipport[0]] = struct{}{}
		logToFile(logFileName, ipport[0])
		fmt.Printf("IP %s exceeded packet limit\n", ip)
	}
}

func logPort(ipport string) {
	port := strings.Split(ipport, ":")
	uniqueMutex.Lock()
	defer uniqueMutex.Unlock()

	if _, exists := uniquePorts[port[1]]; !exists {
		uniquePorts[port[1]] = struct{}{}
		logToFile(logSourcePortFileName, port[1])
	}
}

func saveLogs() {
	uniqueMutex.Lock()
	defer uniqueMutex.Unlock()

	for ip := range uniqueIPs {
		ipport := strings.Split(ip, ":")
		logToFile(logFileName, ipport[0])
		fmt.Printf("IP %s exceeded packet limit\n", ip)
	}

	for port := range uniquePorts {
		logToFile(logSourcePortFileName, port)
	}

	uniqueIPs = make(map[string]struct{})
	uniquePorts = make(map[string]struct{})
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

func portExistsInFile(port string) bool {
	file, err := os.Open(logSourcePortFileName)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == port {
			return true
		}
	}

	return false
}

func logToFile(fileName, entry string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	if ipExistsInFile(entry) {
		return
	} else if portExistsInFile(entry) {
		return
	}

	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(entry + "\n"); err != nil {
		fmt.Println("Error writing to log file:", err)
	}
}

func monitorPackets() {
	for {
		select {
		case <-saveLogsCh:
			saveLogs()
		case <-time.After(time.Second):
			packetMutex.Lock()
			for ip := range packetCounts {
				packetCounts[ip] = 0
			}
			packetMutex.Unlock()
		}
	}
}
