// Provides the channel for the server and client to setup and exchange metadata about the replays
// to run.
package network

import (
    "fmt"
    "net"
    "strconv"
    "strings"

    "wehe-server/internal/clienthandler"
)

const (
    port = 55556
)

type opcode byte // request type from the client

const (
    ask4permission opcode = iota
    mobileStats
)

type responseCode byte // code representing the status of a response back to the client

const (
    okResponse responseCode = iota
    clientInputError
    serverError
)

// Channel that allows client to notify server which replay it would like to run in addition to
// exchanging metadata, like carrier name, GPS info.
type SideChannel struct {
    IP string // IP server should listen on
    Port int // TCP port server should listen on
    ReplayNames []string // names of all the replays
    ConnectedClients *clienthandler.ConnectedClients // connected clients to the side channel
}

func NewSideChannel(ip string, replayNames []string) SideChannel {
    return SideChannel{
        IP: ip,
        Port: port,
        ReplayNames: replayNames,
        ConnectedClients: clienthandler.NewConnectedClients(),
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

// Handles a side channel connection from a clienthandler.
// conn: the client side channel connection
// errChan: the error channel to return any errors
func (sideChannel SideChannel) handleConnection(conn net.Conn) {
    defer conn.Close()

    clt, err := sideChannel.receiveID(conn)
    if err != nil {
        sideChannel.handleSideChannelError(err, clienthandler.Client{})
        return
    }

    majorVersion, err := clt.GetMajorVersionNumber()
    if err != nil {
        sideChannel.handleSideChannelError(err, clt)
        return
    }

    // Wehe version before 4.0 have a different protocol than current Wehe versions
    if majorVersion < 4 {
        err = handleOldSideChannel(conn, clt)
        if err != nil {
            sideChannel.handleSideChannelError(err, clt)
            return
        }
    } else {
        for {
            buffer := make([]byte, 1024)
            n, err := conn.Read(buffer)
            if err != nil {
                sideChannel.handleSideChannelError(err, clt)
                return
            }
            opcode := buffer[0]
            fmt.Println("Got opcode:", opcode)
            switch opcode {
            case byte(ask4permission):
                sideChannel.ask4Permission(clt)
            case byte(mobileStats):
                sideChannel.receiveMobileStats(clt, buffer[:n])
            default:
                sideChannel.handleSideChannelError(fmt.Errorf("Unknown side channel opcode: %d\n", opcode), clt)
            }
        }
    }
}

// Sends a response back to the client.
// clt: the client
// respCode: the status of the response
// message: the information to return the to client
func (sideChannel SideChannel) sendResponse(clt clienthandler.Client, respCode responseCode, message string) {
    messageBytes := []byte(message)
    resp := make([]byte, len(messageBytes) + 1)
    resp[0] = byte(respCode)
    copy(resp[1:], messageBytes)

    clt.Conn.Write(resp)
}


// Handles errors thrown by a side channel connection.
// err: the error that was thrown
// clt: the client handler of the connection that errored
func (sideChannel SideChannel) handleSideChannelError(err error, clt clienthandler.Client) {
    fmt.Println("Side channel error:", err)
    clt.CleanUp(sideChannel.ConnectedClients)
}

// Get the message portion of the bytes received from the clienthandler.
// Bytes received from client should be in format of:
//     first byte: opcode
//     all following bytes: message
// buffer: the bytes received from the client
// n: number bytes received
// Returns the message or any errors
func getMessage(buffer []byte, n int) (string, error) {
    if len(buffer) < 2 {
        return "", fmt.Errorf("Cannot get message from side channel buffer; buffer too short\n")
    }
    return string(buffer[1:n]), nil
}

// Receives the ID declare by the clienthandler.
// conn: the connection to the client
// Returns a information about the client or any errors
func (sideChannel SideChannel) receiveID(conn net.Conn) (clienthandler.Client, error) {
    buffer := make([]byte, 1024)
    _, err := conn.Read(buffer)
    if err != nil {
        return clienthandler.Client{}, err
    }

    pieces := strings.Split(string(buffer), ";")
    if len(pieces) < 6 {
        return clienthandler.Client{}, fmt.Errorf("Expected to receive at least 6 pieces from declare ID; only received %d.\n", len(pieces))
    }

    userID := pieces[0]

    replayIDInt, err := strconv.Atoi(pieces[1])
    if err != nil {
        return clienthandler.Client{}, err
    }
    var replayID clienthandler.ReplayType
    if replayIDInt == 0 {
        replayID = clienthandler.Original
    } else if replayIDInt == 1 {
        replayID = clienthandler.Random
    } else {
        return clienthandler.Client{}, fmt.Errorf("Unexpected replay ID: %d; must be 0 (original) or 1 (random)", replayIDInt)
    }

    //TODO: change client replay files replay names to use _ instead of -, then delete this terrible replace code
    replayName := strings.Replace(pieces[2], "-", "_", -1)

    extraString := pieces[3]
    testID, err := strconv.Atoi(pieces[4])
    if err != nil {
        return clienthandler.Client{}, err
    }
    isLastReplay, err := strToBool(pieces[5])
    if err != nil {
        return clienthandler.Client{}, err
    }

    // Some ISPs may give clients multiple IPs - one for each port. We want to use the test port
    // as the client's public IP. Client may send us an IP, which is the IP of the client using
    // the test port. If client does not provide us with an IP, or if the IP provided is 127.0.0.1,
    // then we just use the side channel IP of the clienthandler.
    publicIP, err := getClientPublicIP(conn)
    if err != nil {
        return clienthandler.Client{}, err
    }
    clientVersion := "1.0"
    if len(pieces) > 6 {
        if pieces[6] != "127.0.0.1" {
            publicIP = pieces[6]
        }
        clientVersion = pieces[7]
    }

    clt := clienthandler.Client{
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

// Determines if client can run replay and seriailzes the response to send back to the client.
// clt: the client handler that made the request
func (sideChannel SideChannel) ask4Permission(clt clienthandler.Client) {
    status, info := clt.Ask4Permission(sideChannel.ReplayNames, sideChannel.ConnectedClients)
    resp := status + ";" + info
    sideChannel.sendResponse(clt, okResponse, resp)
}

func (sideChannel SideChannel) receiveMobileStats(clt clienthandler.Client, buffer []byte) {
    //stats, err := clt.ReceiveMobileStats(clt, message)
}
