// A UDP server to run UDP replays.
package network

import (
    "fmt"
    "net"
    "strings"
)

type UDPServer struct {
    IP string // IP that the server should listen on
    Port int // UDP port that the server should listen on
}

func NewUDPServer(ip string, port int) UDPServer {
    return UDPServer{
        IP: ip,
        Port: port,
    }
}

// Start a UDP server and listen for packets.
// errChan: channel to allow errors to be returned to the main thread
func (udpServer UDPServer) StartServer(errChan chan<- error) {
    conn, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", udpServer.IP, udpServer.Port))
    if err != nil {
        errChan <- err
        return
    }
    defer conn.Close()

    fmt.Println("Listening on UDP", udpServer.Port)
    // get connection from clients
    for {
         buffer := make([]byte, 4096)

        _, addr, err := conn.ReadFrom(buffer)
        if err != nil {
            //TODO: should handle failed test instead of terminating program
            errChan <- err
            return
        }

        errChan2 := make(chan error)
        go udpServer.handleConnection(conn, addr, buffer, errChan2)
        err = <-errChan2
        if err != nil {
            errChan <- err
            return
        }
    }

    errChan <- nil
}

// Handles a UDP connection.
// conn: the UDP connection
// addr: the client IP and port
// buffer: the content received from the client
// errChan: channel to send back any errors
func (udpServer UDPServer) handleConnection(conn net.PacketConn, addr net.Addr, buffer []byte, errChan chan<- error) {
    //TODO: figure this out https://github.com/NEU-SNS/wehe-py3/blob/master/src/replay_server.py#L324

    clientIP := strings.Split(addr.String(), ":")[0]
    // return client IP address if it asks for it
    if strings.HasPrefix(string(buffer), "WHATSMYIPMAN") {
        _, err := conn.WriteTo([]byte(clientIP), addr)
        if err != nil {
            errChan <- err
            return
        }
    }
    errChan <- nil
}
