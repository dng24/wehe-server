// A TCP server to run TCP replays.
package network


import (
    "fmt"
    "net"
    "strings"
    "time"

    "wehe-server/internal/clienthandler"
    "wehe-server/internal/testdata"
)

type TCPServer struct {
    IP string // IP that the server should listen on
    Port int // TCP port that the server should listen on
    IPReplayNameMapping *clienthandler.ConnectedClients // map of client IPs that are connected to the side channel to the replay name client wants to run
}

func NewTCPServer(ip string, port int, ipReplayNameMapping *clienthandler.ConnectedClients) TCPServer {
    return TCPServer{
        IP: ip,
        Port: port,
        IPReplayNameMapping: ipReplayNameMapping,
    }
}

// Start a TCP server and listen for connections.
// errChan: channel to allow errors to be returned to the main thread
func (tcpServer TCPServer) StartServer(errChan chan<- error) {
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", tcpServer.IP, tcpServer.Port))
    if err != nil {
        errChan <- err
        return
    }
    defer listener.Close()

    fmt.Println("Listening on TCP", tcpServer.Port)
    // get connections from clients
    for {
        conn, err := listener.Accept()
        if err != nil {
            //TODO: figure out what to do if connection can't be accepted
            fmt.Println("Error accepting connection:", err)
            continue
        }

        //TODO: figure out what to do when this errors and how to wait for error without blocking
        go tcpServer.handleConnection(conn)
    }

    errChan <- nil
}

func (tcpServer TCPServer) handleConnection(conn net.Conn) {
    defer conn.Close()

    //TODO: figure this out https://github.com/NEU-SNS/wehe-py3/blob/master/src/replay_server.py#L324

    buffer := make([]byte, 4096)

    // reads GET request to WHATSMYIPMAN or the first packet of the replay from client
    numBytes, err := conn.Read(buffer)
    if err != nil {
        tcpServer.handleTCPError(fmt.Errorf("Unable to read buffer from connection: %v", err))
        return
    }

    addr, ok := conn.RemoteAddr().(*net.TCPAddr)
    if !ok {
        tcpServer.handleTCPError(fmt.Errorf("Unable to get client IP."))
        return
    }
    clientIP := addr.IP.String()

    // TODO: probably should compare bytes instead of converting to string
    // return client IP address if it asks for it
    if strings.HasPrefix(string(buffer), "GET /WHATSMYIPMAN") || strings.HasPrefix(string(buffer), "WHATSMYIPMAN") {
        _, err = conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n" + clientIP))
        if err != nil {
            tcpServer.handleTCPError(err)
        }
        return
    }

    replayName, err := tcpServer.IPReplayNameMapping.Get(clientIP)
    if err != nil {
        tcpServer.handleTCPError(err)
        return
    }

    // get the replay packets and info
    replayInfo, err := testdata.ParseReplayJSON(replayName)
    if err != nil {
        tcpServer.handleTCPError(err)
        return
    }

    // each response set contains packets that should be sent after server receives a certain number of bytes from client
    // TODO: add hash checkigng?
    for i, response := range replayInfo.Responses {
        responseSet := response.(testdata.TCPResponseSet)
        for {
            if numBytes >= responseSet.RequestLength {
                break
            }
            nBytes, err := conn.Read(buffer)
            if err != nil {
                tcpServer.handleTCPError(err)
                return
            }
            fmt.Printf("Received %d bytes from client.\n", nBytes)
            numBytes += nBytes
        }
        numBytes = 0

        startTime := time.Now()
        // send each packet in the response set
        for _, packet := range responseSet.Packets {
            if !tcpServer.IPReplayNameMapping.Has(clientIP) {
                return
            }
            if timing {
                time.Sleep(startTime.Add(packet.Timestamp).Sub(time.Now()))
            }

            fmt.Printf("Sending response to packet %d at %s\n", i + 1, packet.Timestamp)
            _, err = conn.Write(packet.Payload)
            if err != nil {
                tcpServer.handleTCPError(err)
                return
            }
        }
    }
}

func (tcpServer TCPServer) handleTCPError(err error) {
    fmt.Println("TCP connection error:", err)
}
