// Provides the channel for the server and client to setup and exchange metadata about the replays
// to run.
package network

import (
    "fmt"
    "net"
)

const (
    port = 55556
)

// Channel that allows client to notify server which replay it would like to run in addition to
// exchanging metadata, like carrier name, GPS info.
type SideChannel struct {
    IP string // IP server should listen on
    Port int // TCP port server should listen on
}

func NewSideChannel(ip string) SideChannel {
    return SideChannel{
        IP: ip,
        Port: port,
    }
}

// Starts the side channel server and listen for client connections.
// errChan: channel used to communicate errors back to the main thread
func (sideChannel SideChannel) StartServer(errChan chan<- error) {
    // TODO: figure out tls
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", sideChannel.IP, sideChannel.Port))
    if err != nil {
        errChan <- err
        return
    }
    defer listener.Close()

    fmt.Println("Listening on side channel", sideChannel.Port)
    // get connections from clients
    for {
        conn, err := listener.Accept()
        if err != nil {
            //TODO: figure out what should happen if connection can't be accepted
            fmt.Println("Error accepting connection:", err)
            continue
        }

        go sideChannel.handleConnection(conn)
    }

    errChan <- nil
}

func (sideChannel SideChannel) handleConnection(conn net.Conn) {
    defer conn.Close()

}
