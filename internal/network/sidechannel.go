// Provides the channel for the server and client to setup and exchange metadata about the replays
// to run.
package network

import (
    "fmt"
    "net"
    "strconv"
    "strings"
)

const (
    port = 55556
)

type ReplayType int

const (
    Original ReplayType = iota
    Random
)

// The struct that is received on initial contact from the client
type ClientInfo struct {
    userID string // the 10 character user ID
    replayID ReplayType // indicates whether replay is the original or random replay
    replayName string // name of the replay to run
    extraString string // extra information; in the current version, it is number attempts client makes to MLab before successful connection
    testID int // the ID of the test for the particular user
    isLastReplay bool // true if this is the last replay of the test; false otherwise
    publicIP string // public IP of the client retrieved from the test port
    clientVersion string // client version number of Wehe
}

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

    sideChannel.receiveID(conn)
}

// Receives the ID declare by the client.
// conn: the connection to the client
// Returns a information about the client or any errors
func (sideChannel SideChannel) receiveID(conn net.Conn) (ClientInfo, error) {
    buffer := make([]byte, 1024)
    _, err := conn.Read(buffer)
    if err != nil {
        return ClientInfo{}, err
    }

    pieces := strings.Split(string(buffer), ";")
    if len(pieces) < 6 {
        return ClientInfo{}, fmt.Errorf("Expected to receive at least 6 pieces from declare ID; only received %d.\n", len(pieces))
    }

    userID := pieces[0]

    replayIDInt, err := strconv.Atoi(pieces[1])
    if err != nil {
        return ClientInfo{}, err
    }
    var replayID ReplayType
    if replayIDInt == 0 {
        replayID = Original
    } else if replayIDInt == 1 {
        replayID = Random
    } else {
        return ClientInfo{}, fmt.Errorf("Unexpected replay ID: %d; must be 0 (original) or 1 (random)", replayIDInt)
    }

    replayName := pieces[2]
    extraString := pieces[3]
    testID, err := strconv.Atoi(pieces[4])
    if err != nil {
        return ClientInfo{}, err
    }
    isLastReplay, err := strToBool(pieces[5])
    if err != nil {
        return ClientInfo{}, err
    }

    // Some ISPs may give clients multiple IPs - one for each port. We want to use the test port
    // as the client's public IP. Client may send us an IP, which is the IP of the client using
    // the test port. If client does not provide us with an IP, or if the IP provided is 127.0.0.1,
    // then we just use the side channel IP of the client.
    publicIP, err := getClientPublicIP(conn)
    if err != nil {
        return ClientInfo{}, err
    }
    clientVersion := "1.0"
    if len(pieces) > 6 {
        if pieces[6] != "127.0.0.1" {
            publicIP = pieces[6]
        }
        clientVersion = pieces[7]
    }

    clientInfo := ClientInfo{
        userID: userID,
        replayID: replayID,
        replayName: replayName,
        extraString: extraString,
        testID: testID,
        isLastReplay: isLastReplay,
        publicIP: publicIP,
        clientVersion: clientVersion,
    }

    fmt.Println(clientInfo)
    return clientInfo, nil
}

// Converts a string to boolean.
// str: the string to convert into a bool
// Returns a bool or any errors
func strToBool(str string) (bool, error) {
    lowerStr := strings.ToLower(str)
    if lowerStr == "true" {
        return true, nil
    } else if lowerStr == "false" {
        return false, nil
    } else {
        return false, fmt.Errorf("Cannot parse '%s' into a bool\n", str)
    }
}

// Gets the client IP of a connection.
// conn: the client connection
// Returns the client IP or any erros
func getClientPublicIP(conn net.Conn) (string, error) {
    remoteAddr := conn.RemoteAddr().String()

    host, _, err := net.SplitHostPort(remoteAddr)
    if err != nil {
        return "", err
    }

    ip := net.ParseIP(host)
    if ip == nil {
        return "", fmt.Errorf("invalid IP address: %s", host)
    }

    return ip.String(), nil
}
