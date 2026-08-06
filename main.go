package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

var cache = sync.Map{}

func main() {
	log.Println("Starting MiniRedis Server on :6379...")

	listener, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		log.Fatalf("Failed to bind TCP port 6379: %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		cmd, args, err := parseRESP(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			conn.Write([]byte(fmt.Sprintf("-ERR %v\r\n", err)))
			continue
		}

		response := executeCommand(cmd, args)
		conn.Write([]byte(response))
	}
}

// Parses Redis Serialization Protocol (RESP)
func parseRESP(reader *bufio.Reader) (string, []string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", nil, err
	}

	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return "", nil, fmt.Errorf("empty request")
	}

	// RESP arrays start with '*'
	if line[0] != '*' {
		return "", nil, fmt.Errorf("invalid protocol start token: %c", line[0])
	}

	arrayLen, err := strconv.Atoi(line[1:])
	if err != nil {
		return "", nil, fmt.Errorf("invalid array length")
	}

	args := make([]string, 0, arrayLen)
	for i := 0; i < arrayLen; i++ {
		// Read bulk string length (starts with '$')
		bulkLenLine, err := reader.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		bulkLenLine = strings.TrimSpace(bulkLenLine)
		if bulkLenLine[0] != '$' {
			return "", nil, fmt.Errorf("expected bulk string token '$'")
		}

		bulkLen, err := strconv.Atoi(bulkLenLine[1:])
		if err != nil {
			return "", nil, fmt.Errorf("invalid bulk string length")
		}

		// Read bulk string content + trailing CRLF
		buf := make([]byte, bulkLen+2)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return "", nil, err
		}

		args = append(args, string(buf[:bulkLen]))
	}

	if len(args) == 0 {
		return "", nil, fmt.Errorf("empty command arguments")
	}

	return strings.ToUpper(args[0]), args[1:], nil
}

func executeCommand(cmd string, args []string) string {
	switch cmd {
	case "PING":
		return "+PONG\r\n"
	case "SET":
		if len(args) < 2 {
			return "-ERR wrong number of arguments for 'set' command\r\n"
		}
		cache.Store(args[0], args[1])
		return "+OK\r\n"
	case "GET":
		if len(args) < 1 {
			return "-ERR wrong number of arguments for 'get' command\r\n"
		}
		val, ok := cache.Load(args[0])
		if !ok {
			return "$-1\r\n" // Nil response
		}
		strVal := val.(string)
		return fmt.Sprintf("$%d\r\n%s\r\n", len(strVal), strVal)
	case "DEL":
		if len(args) < 1 {
			return "-ERR wrong number of arguments for 'del' command\r\n"
		}
		deletedCount := 0
		for _, key := range args {
			if _, ok := cache.LoadAndDelete(key); ok {
				deletedCount++
			}
		}
		return fmt.Sprintf(":%d\r\n", deletedCount)
	case "EXISTS":
		if len(args) < 1 {
			return "-ERR wrong number of arguments for 'exists' command\r\n"
		}
		existsCount := 0
		for _, key := range args {
			if _, ok := cache.Load(key); ok {
				existsCount++
			}
		}
		return fmt.Sprintf(":%d\r\n", existsCount)
	default:
		return fmt.Sprintf("-ERR unknown command '%s'\r\n", cmd)
	}
}
