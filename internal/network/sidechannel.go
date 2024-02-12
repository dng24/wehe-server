// Provides the channel for the server and client to setup and exchange metadata about the replays
// to run.
package network

import (
    "fmt"
    "net"
    "strconv"
    "strings"

    "wehe-server/internal/client"
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

        errChan2 := make(chan error)
        go sideChannel.handleConnection(conn, errChan2)
        err = <-errChan2
        if err != nil {
            fmt.Println("Side channel error:", err)
        }
    }

    errChan <- nil
}

// Handles a side channel connection from a client.
// conn: the client side channel connection
// errChan: the error channel to return any errors
func (sideChannel SideChannel) handleConnection(conn net.Conn, errChan chan<- error) {
    defer conn.Close()

    clt, err := sideChannel.receiveID(conn)
    if err != nil {
        errChan <- err
        return
    }

    majorVersion, err := clt.GetMajorVersionNumber()
    if err != nil {
        errChan <- err
        return
    }

    // Wehe version before 4.0 have a different protocol than current Wehe versions
    if majorVersion < 4 {
        err = handleOldSideChannel(conn, clt)
        if err != nil {
            errChan <- err
            return
        }
    } else {
        for {
            buffer := make([]byte, 1024)
            _, err := conn.Read(buffer)
            if err != nil {
                errChan <- err
                return
            }
            opcode := buffer[0]
            switch opcode {
            case 2:
                clt.Ask4Permission()
            case 3:
                message, err := getMessage(buffer)
                if err != nil {
                    errChan <- err
                    return
                }
                clt.ReceiveDeviceInfo(message)
            default:
                errChan <- fmt.Errorf("Unknown side channel opcode: %d\n", opcode)
            }
        }
    }
    errChan <- nil
}

// Get the message portion of the bytes received from the client.
// Bytes received from client should be in format of:
//     first byte: opcode
//     all following bytes: message
// buffer: the bytes received from the client
// Returns the message or any errors
func getMessage(buffer []byte) (string, error) {
    if len(buffer) < 2 {
        return "", fmt.Errorf("Cannot get message from side channel buffer; buffer too short\n")
    }
    return string(buffer[1:]), nil
}

// Receives the ID declare by the client.
// conn: the connection to the client
// Returns a information about the client or any errors
func (sideChannel SideChannel) receiveID(conn net.Conn) (client.Client, error) {
    buffer := make([]byte, 1024)
    _, err := conn.Read(buffer)
    if err != nil {
        return client.Client{}, err
    }

    pieces := strings.Split(string(buffer), ";")
    if len(pieces) < 6 {
        return client.Client{}, fmt.Errorf("Expected to receive at least 6 pieces from declare ID; only received %d.\n", len(pieces))
    }

    userID := pieces[0]

    replayIDInt, err := strconv.Atoi(pieces[1])
    if err != nil {
        return client.Client{}, err
    }
    var replayID client.ReplayType
    if replayIDInt == 0 {
        replayID = client.Original
    } else if replayIDInt == 1 {
        replayID = client.Random
    } else {
        return client.Client{}, fmt.Errorf("Unexpected replay ID: %d; must be 0 (original) or 1 (random)", replayIDInt)
    }

    replayName := pieces[2]
    extraString := pieces[3]
    testID, err := strconv.Atoi(pieces[4])
    if err != nil {
        return client.Client{}, err
    }
    isLastReplay, err := strToBool(pieces[5])
    if err != nil {
        return client.Client{}, err
    }

    // Some ISPs may give clients multiple IPs - one for each port. We want to use the test port
    // as the client's public IP. Client may send us an IP, which is the IP of the client using
    // the test port. If client does not provide us with an IP, or if the IP provided is 127.0.0.1,
    // then we just use the side channel IP of the client.
    publicIP, err := getClientPublicIP(conn)
    if err != nil {
        return client.Client{}, err
    }
    clientVersion := "1.0"
    if len(pieces) > 6 {
        if pieces[6] != "127.0.0.1" {
            publicIP = pieces[6]
        }
        clientVersion = pieces[7]
    }

    clt := client.Client{
        Conn: conn,
        UserID: userID,
        ReplayID: replayID,
        ReplayName: replayName,
        ExtraString: extraString,
        TestID: testID,
        IsLastReplay: isLastReplay,
        PublicIP: publicIP,
        ClientVersion: clientVersion,
    }

    fmt.Println(clt)
    return clt, nil
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
