// Provides the channel for the server and client to setup and exchange metadata about the replays
// to run.
package network

import (
    "crypto/tls"
    "encoding/binary"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "strconv"
    "strings"

    "github.com/m-lab/uuid"

    "wehe-server/internal/clienthandler"
)

const (
    port = 55556
)

type opcode byte // request type from the client

const (
    invalid opcode = 255
    oldDeclareID opcode = 0x30 // first byte of old protocol is 0x30, so we use that to detect it
    receiveID opcode = iota
    ask4permission
    mobileStats
    throughputs
    declareReplay
    analyzeTest
)

type responseCode byte // code representing the status of a response back to the client

const (
    okResponse responseCode = iota
    errorResponse
)

// Channel that allows client to notify server which replay it would like to run in addition to
// exchanging metadata, like carrier name, GPS info.
type SideChannel struct {
    IP string // IP server should listen on
    Port int // TCP port server should listen on
    ReplayNames []string // names of all the replays
    ConnectedClients *clienthandler.ConnectedClients // connected clients to the side channel
    TmpResultsDir string // the directory to write temporary files to
    ResultsDir string // the directory to write permanent results to
}

func NewSideChannel(ip string, replayNames []string, uuidPrefixFile string, tmpResultsDir string, resultsDir string) (SideChannel, error) {
    err := uuid.SetUUIDPrefixFile(uuidPrefixFile)
    if err != nil {
        return SideChannel{}, err
    }
    return SideChannel{
        IP: ip,
        Port: port,
        ReplayNames: replayNames,
        ConnectedClients: clienthandler.NewConnectedClients(),
        TmpResultsDir: tmpResultsDir,
        ResultsDir: resultsDir,
    }, nil
}

// Starts the side channel server and listen for client connections.
// cert: the server cert
// errChan: channel used to communicate errors back to the main thread
func (sideChannel SideChannel) StartServer(cert tls.Certificate, errChan chan<- error) {
    tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
    listener, err := tls.Listen("tcp", fmt.Sprintf("%s:%d", sideChannel.IP, sideChannel.Port), tlsConfig)
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
func (sideChannel SideChannel) handleConnection(conn net.Conn) {
    defer conn.Close()
    var clt *clienthandler.Client
    // TODO: add feature that forces user to upgrade if their version is too old

    for {
        op, first4Bytes, message, err := sideChannel.readRequest(conn)
        if err != nil {
            // when client disconnects, an error is thrown, but that isn't really an error
            if err != io.EOF && !strings.Contains(err.Error(), "tls: user canceled") {
                handleSideChannelError(err)
            }
            break
        }
        fmt.Println("Got opcode:", op)

        if clt == nil && op != oldDeclareID && op != receiveID {
            handleSideChannelError(fmt.Errorf("Client is nil. Was test ever requested?\n"))
            break
        }

        switch op {
        case oldDeclareID:
            err = sideChannel.handleOldSideChannel(conn, first4Bytes)
        case receiveID:
            clt, err = sideChannel.receiveID(conn, message)
            if err == nil {
                defer clt.CleanUp(sideChannel.ConnectedClients)
            }
        case ask4permission:
            err = sideChannel.ask4Permission(clt)
        case mobileStats:
            err = sideChannel.receiveMobileStats(clt, message)
        case throughputs:
            err = sideChannel.receiveThroughputs(clt, message)
            if err == nil {
                err = clt.WriteReplayInfoToFile(sideChannel.TmpResultsDir)
            }
        case declareReplay:
            err = sideChannel.declareReplay(clt, message)
        case analyzeTest:
            err = sideChannel.analyzeTest(clt)
            /*if err != nil {
                break
            }
            err = clt.WriteResultsToFile()
            if err == nil {
                err = clt.WriteReplayInfoToFile(sideChannel.TmpResultsDir)
            }*/
        default:
            err = fmt.Errorf("Unknown side channel opcode: %d\n", op)
        }

        if err != nil {
            handleSideChannelError(err)
            break
        }
    }
}

// Handles errors thrown by a side channel connection.
// err: the error that was thrown
func handleSideChannelError(err error) {
    // TODO: this should be logged to error file
    fmt.Println("Side channel error:", err)
}

// Reads a request from the client. First, an 8-bit opcode and 24-bit big-endian unsigned message
// length is read. Using this length, the acutal message is then read.
// conn: the connection to the client
// Returns the opcode, first 4 bytes read (if old protocol), message read, and any errors
func (sideChannel SideChannel) readRequest(conn net.Conn) (opcode, []byte, string, error) {
    // get opcode and size of message
    opcodeAndDataLength := make([]byte, 4)
    _, err := io.ReadFull(conn, opcodeAndDataLength)
    if err != nil {
        return opcode(invalid), nil, "", err
    }
    op := opcode(opcodeAndDataLength[0])
    if op == oldDeclareID {
        // if old protocol is used, return the data that has been read so far and let the old
        // protocol functions handle everything
        return op, opcodeAndDataLength, "", nil
    }
    opcodeAndDataLength[0] = 0 // zero out first byte so that 24-bit length can be read with Uint32
    dataLength := binary.BigEndian.Uint32(opcodeAndDataLength)

    // get the message
    message := make([]byte, dataLength)
    _, err = io.ReadFull(conn, message)
    if err != nil {
        return opcode(invalid), nil, "", err
    }
    return op, nil, string(message), nil
}

// Sends a response back to the client.
// clt: the client
// respCode: the status of the response
// message: the information to return the to client
// Returns any errors
func (sideChannel SideChannel) sendResponse(clt *clienthandler.Client, respCode responseCode, message string) error {
    messageBytes := []byte(message)
    messageLength := len(messageBytes) + 1

    // send size of message
    messageLengthBytes := make([]byte, 4)
    binary.BigEndian.PutUint32(messageLengthBytes, uint32(messageLength))
    _, err := clt.Conn.Write(messageLengthBytes)
    if err != nil {
        return err
    }

    // send the message
    resp := make([]byte, messageLength)
    resp[0] = byte(respCode)
    copy(resp[1:], messageBytes)

    _, err = clt.Conn.Write(resp)
    if err != nil {
        return err
    }
    return nil
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

// Receives information about the test the client has requested to run.
// conn: the connection to the client
// message: information about the test requested to be run
// Returns a information about the client or any errors
func (sideChannel SideChannel) receiveID(conn net.Conn, message string) (*clienthandler.Client, error) {
    pieces := strings.Split(message, ";")
    if len(pieces) < 6 {
        return nil, fmt.Errorf("Expected to receive at least 6 pieces from declare ID; only received %d.\n", len(pieces))
    }

    userID := pieces[0]

    replayIDInt, err := strconv.Atoi(pieces[1])
    if err != nil {
        return nil, err
    }
    var replayID clienthandler.ReplayType
    if replayIDInt == 0 {
        replayID = clienthandler.Original
    } else if replayIDInt == 1 {
        replayID = clienthandler.Random
    } else {
        return nil, fmt.Errorf("Unexpected replay ID: %d; must be 0 (original) or 1 (random)", replayIDInt)
    }

    //TODO: change client replay files replay names to use _ instead of -, then delete this terrible replace code
    replayName := strings.Replace(pieces[2], "-", "_", -1)

    extraString := pieces[3]
    testID, err := strconv.Atoi(pieces[4])
    if err != nil {
        return nil, err
    }
    isLastReplay, err := strToBool(pieces[5])
    if err != nil {
        return nil, err
    }

    // Some ISPs may give clients multiple IPs - one for each port. We want to use the test port
    // as the client's public IP. Client may send us an IP, which is the IP of the client using
    // the test port. If client does not provide us with an IP, or if the IP provided is 127.0.0.1,
    // then we just use the side channel IP of the clienthandler.
    publicIP, err := getClientPublicIP(conn)
    if err != nil {
        return nil, err
    }
    clientVersion := "1.0"
    if len(pieces) > 6 {
        if pieces[6] != "127.0.0.1" {
            publicIP = pieces[6]
        }
        clientVersion = pieces[7]
    }

    tlsConn, ok := conn.(*tls.Conn)
    if !ok {
        return nil, fmt.Errorf("Side Channel expected to be TLS connection; it is not\n")
    }
    tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
    if !ok {
        return nil, fmt.Errorf("Side Channel expected to be TCP connection; it is not\n")
    }
    mlabUUID, err := uuid.FromTCPConn(tcpConn)
    if err != nil {
        return nil, err
    }

    clt := clienthandler.NewClient(conn, userID, extraString, testID, publicIP, clientVersion, mlabUUID)
    clt.AddReplay(replayID, replayName, isLastReplay)

    fmt.Println(clt)
    return clt, nil
}

// Determines if client can run replay and seriailzes the response to send back to the client.
// clt: the client handler that made the request
// Returns any errors
func (sideChannel SideChannel) ask4Permission(clt *clienthandler.Client) error {
    status, info, err := clt.Ask4Permission(sideChannel.ReplayNames, sideChannel.ConnectedClients)
    if err != nil {
        return err
    }
    resp := status + ";" + info
    err = sideChannel.sendResponse(clt, okResponse, resp)
    if err != nil {
        return err
    }
    return nil
}

// Receives device, network, and location information about the client.
// clt: the client handler that made the request
// message: json information about the client
// Returns any errors
func (sideChannel SideChannel) receiveMobileStats(clt *clienthandler.Client, message string) error {
    err := clt.ReceiveMobileStats(message)
    if err != nil {
        sideChannel.sendResponse(clt, errorResponse, "")
        return err
    }
    err = sideChannel.sendResponse(clt, okResponse, "")
    if err != nil {
        return err
    }
    return nil
}

// Receives replay duration, the throughputs, and sample times from a replay.
// clt: the client handler that made the request
// message: the data received from the client
// Returns any errors
func (sideChannel SideChannel) receiveThroughputs(clt *clienthandler.Client, message string) error {
    err := clt.ReceiveThroughputs(message, sideChannel.TmpResultsDir)
    if err != nil {
        sideChannel.sendResponse(clt, errorResponse, "")
        return err
    }
    err = sideChannel.sendResponse(clt, okResponse, "")
    if err != nil {
        return err
    }
    return nil
}

// Receives request to run an additional replay and determines if that replay is allowed to run.
// clt: the client handler that made the request
// message: the data received from the client
// Returns any errors
func (sideChannel SideChannel) declareReplay(clt *clienthandler.Client, message string) error {
    status, info, err := clt.DeclareReplay(sideChannel.ReplayNames, message)
    if err != nil {
        return err
    }
    resp := status + ";" + info
    err = sideChannel.sendResponse(clt, okResponse, resp)
    if err != nil {
        return err
    }
    return nil
}

// The stats to send back to the client for a 2-sample KS test analysis
type KS2Result struct {
    Area0var float64 `json:"Area0Var"`
    KS2pVal float64 `json:"KS2pVal"`
    OriginalAvgThroughput float64 `json:"OriginalAvgThroughput"`
    RandomAvgThroughput float64 `json:"RandomAvgThroughput"`
}

// Performs a 2-sample KS test.
// clt: the client handler that made the request
// Returns any errors
func (sideChannel SideChannel) analyzeTest(clt *clienthandler.Client) error {
    err := clt.AnalyzeTest()
    if err != nil {
        sideChannel.sendResponse(clt, errorResponse, "")
        return err
    }
    ks2Result := KS2Result{
        Area0var: clt.Analysis.Area0var,
        KS2pVal: clt.Analysis.KS2pVal,
        OriginalAvgThroughput: clt.Analysis.OriginalReplayStats.Average,
        RandomAvgThroughput: clt.Analysis.RandomReplayStats.Average,
    }
    jsonBytes, err := json.Marshal(ks2Result)
    if err != nil {
        return err
    }

    err = sideChannel.sendResponse(clt, okResponse, string(jsonBytes))
    if err != nil {
        return err
    }
    return nil
}
