// A UDP server to run UDP replays.
package network

import (
    "fmt"
    "net"
    "strings"
    "time"

    "wehe-server/internal/replay"
)

const (
    udpReplayTimeout = 45 * time.Second // each UDP replay is limited to 45 seconds so that user doesn't have to wait forever
)

type UDPServer struct {
    IP string // IP that the server should listen on
    Port int // UDP port that the server should listen on
    ConnectedIPs map[string]struct{} // set of IPs of the connected clients
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
    // TODO: probably should compare bytes instead of converting buffer to string
    // return client IP address if it asks for it
    if strings.HasPrefix(string(buffer), "WHATSMYIPMAN") {
        _, err := conn.WriteTo([]byte(clientIP), addr)
        if err != nil {
            errChan <- err
            return
        }
//        errChan <- nil
//        return
    }

    _, exists := udpServer.ConnectedIPs[clientIP]
    if !exists {
        packets := make([]replay.Packet, 0)
        err := udpServer.sendPackets(conn, addr, packets, time.Now(), true) //TODO fix timing once replay files are read in
        if err != nil {
            errChan <- err
            return
        }
    }

    errChan <- nil
}

// Sends UDP packets to the client.
// conn: UDP connection to client
// addr: the client IP and port
// packets: the packets to send to the client
// startTime: the start time of the replay (time when first packet received from client)
// timing: true if packets should be sent at their timestamp; false otherwise
// Returns any errors
func (udpServer UDPServer) sendPackets(conn net.PacketConn, addr net.Addr, packets []replay.Packet, startTime time.Time, timing bool) error {
    for _, packet := range packets {
        // replays stop after a certain amount of time so that user doesn't have to wait too long
        elapsedTime := time.Now().Sub(startTime)
        if elapsedTime > udpReplayTimeout {
            break
        }

        // allows packets to be sent at the time of the timestamp
        if timing {
            time.Sleep(startTime.Add(packet.Timestamp).Sub(time.Now()))
        }

        _, err := conn.WriteTo(packet.Payload, addr)
        if err != nil {
            return err
        }
    }

    return nil
}
