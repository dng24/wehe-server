// A UDP server to run UDP replays.
package network

import (
    "fmt"
    "net"
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
    listener, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", udpServer.IP, udpServer.Port))
    if err != nil {
        errChan <- err
        return
    }
    defer listener.Close()

    fmt.Println("Listening on UDP", udpServer.Port)
    // get connection from clients
    for {
         buffer := make([]byte, 4096)

        _, addr, err := listener.ReadFrom(buffer)
        if err != nil {
            //TODO: should handle failed test instead of terminating program
            errChan <- err
            return
        }

        go udpServer.handleConnection(buffer, addr.String())
    }

    errChan <- nil
}

func (udpServer UDPServer) handleConnection(buffer []byte, clientIP string) {
    //TODO: figure this out https://github.com/NEU-SNS/wehe-py3/blob/master/src/replay_server.py#L324

    fmt.Println("UDP client IP:", clientIP)
}
