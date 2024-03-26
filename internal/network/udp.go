// A UDP server to run UDP replays.
package network

import (
    "fmt"
    "net"
    "strings"
    "time"

    "wehe-server/internal/clienthandler"
    "wehe-server/internal/testdata"
)

const (
    udpReplayTimeout = 45 * time.Second // each UDP replay is limited to 45 seconds so that user doesn't have to wait forever
)

type UDPServer struct {
    IP string // IP that the server should listen on
    Port int // UDP port that the server should listen on
    ConnectedIPs map[string]struct{} // set of IPs of the connected clients TODO: does this need mutex??? probably
    IPReplayNameMapping *clienthandler.ConnectedClients // map of client IPs that are connected to the side channel to the replay name client wants to run
}

func NewUDPServer(ip string, port int, ipReplayNameMapping *clienthandler.ConnectedClients) UDPServer {
    return UDPServer{
        IP: ip,
        Port: port,
        ConnectedIPs: make(map[string]struct{}),
        IPReplayNameMapping: ipReplayNameMapping,
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

        numBytes, addr, err := conn.ReadFrom(buffer)
        if err != nil {
            //TODO: should handle failed test instead of terminating program
            errChan <- err
            return
        }

        go udpServer.handleConnection(conn, addr, buffer[:numBytes])
    }

    errChan <- nil
}

// Handles a UDP connection.
// conn: the UDP connection
// addr: the client IP and port
// buffer: the content received from the client
func (udpServer UDPServer) handleConnection(conn net.PacketConn, addr net.Addr, buffer []byte) {
    //TODO: figure this out https://github.com/NEU-SNS/wehe-py3/blob/master/src/replay_server.py#L324

    clientIP := strings.Split(addr.String(), ":")[0]
    // TODO: probably should compare bytes instead of converting buffer to string
    // return client IP address if it asks for it
    if strings.HasPrefix(string(buffer), "WHATSMYIPMAN") {
        _, err := conn.WriteTo([]byte(clientIP), addr)
        if err != nil {
            udpServer.handleUDPError(err)
        }
        return
    }

    _, exists := udpServer.ConnectedIPs[clientIP]
    if !exists {
        udpServer.ConnectedIPs[clientIP] = struct{}{}
        defer delete(udpServer.ConnectedIPs, clientIP)

        replayName, err := udpServer.IPReplayNameMapping.Get(clientIP)
        if err != nil {
            udpServer.handleUDPError(err)
            return
        }

        // TODO: optimize so that replays can stay in ram for more than 1 client
        replayInfo, err := testdata.ParseReplayJSON(replayName)
        if err != nil {
            udpServer.handleUDPError(err)
            return
        }
        err = udpServer.sendPackets(conn, addr, clientIP, replayInfo.Responses, time.Now(), true) //TODO fix timing once replay files are read in
        if err != nil {
            udpServer.handleUDPError(err)
            return
        }
    } else {
        fmt.Printf("Received %d bytes from client.\n", len(buffer))
    }
}

// Handles errors thrown by a UDP connection.
// err: the error that was thrown
func (udpServer UDPServer) handleUDPError(err error) {
    fmt.Println("UDP conection error:", err)
}

// Sends UDP packets to the client.
// conn: UDP connection to client
// addr: the client IP and port
// packets: the packets to send to the client
// startTime: the start time of the replay (time when first packet received from client)
// timing: true if packets should be sent at their timestamps; false otherwise
// Returns any errors
func (udpServer UDPServer) sendPackets(conn net.PacketConn, addr net.Addr, clientIP string, packets []testdata.Response, startTime time.Time, timing bool) error {
    packetLen := len(packets)
    for i, p := range packets {
        // check to make sure client is still connected to server before continuing
        if !udpServer.IPReplayNameMapping.Has(clientIP) {
            break
        }
        packet := p.(testdata.UDPPacket)
        // replays stop after a certain amount of time so that user doesn't have to wait too long
        elapsedTime := time.Now().Sub(startTime)
        if elapsedTime > udpReplayTimeout {
            break
        }

        // allows packets to be sent at the time of the timestamp
        if timing {
            time.Sleep(startTime.Add(packet.Timestamp).Sub(time.Now()))
        }

        fmt.Printf("Sending packet %d/%d at %s\n", i + 1, packetLen, packet.Timestamp)
        _, err := conn.WriteTo(packet.Payload, addr)
        if err != nil {
            return err
        }
    }

    return nil
}
