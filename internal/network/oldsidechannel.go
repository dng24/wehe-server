// Handles side channel connections from Wehe clients < v4.0
package network

import (
    "fmt"
    "io"
    "net"
    "strconv"
    "strings"

    "github.com/m-lab/uuid"

    "wehe-server/internal/clienthandler"
)

const (
    // This list contains the information for all the replays supported by the old server
    // This needs to be modified if new replays are added to the old server
    serverMapping = "{'tcp': {'': {'00000': ['', 34081]}, '002.021.034.145': {'00443': ['', 443]}, '003.162.003.119': {'00443': ['', 443]}, '008.249.245.246': {'00080': ['', 80]}, '008.252.208.244': {'00443': ['', 443]}, '013.225.025.052': {'00443': ['', 443]}, '017.253.011.202': {'00080': ['', 80]}, '018.002.192.002': {'00443': ['', 443]}, '018.032.197.018': {'00443': ['', 443]}, '018.160.041.126': {'00443': ['', 443]}, '023.015.179.224': {'00443': ['', 443]}, '023.033.029.087': {'00443': ['', 443]}, '023.040.060.072': {'00443': ['', 443]}, '023.040.060.146': {'00443': ['', 443]}, '023.040.060.160': {'00443': ['', 443]}, '023.197.180.251': {'00443': ['', 443]}, '035.241.016.093': {'00443': ['', 443]}, '045.057.062.168': {'00443': ['', 443]}, '052.223.227.060': {'00443': ['', 443]}, '052.223.227.181': {'00443': ['', 443]}, '065.158.047.083': {'00080': ['', 80]}, '074.125.172.072': {'00443': ['', 443]}, '082.216.034.026': {'00443': ['', 443]}, '082.216.034.032': {'00443': ['', 443]}, '093.017.156.102': {'00443': ['', 443]}, '139.104.212.047': {'00443': ['', 443]}, '147.160.181.042': {'00443': ['', 443]}, '151.101.118.248': {'00443': ['', 443]}, '151.101.248.246': {'00080': ['', 80]}, '151.101.250.109': {'00443': ['', 443]}, '157.240.245.063': {'00443': ['', 443]}, '172.217.129.041': {'00443': ['', 443]}, '188.065.126.005': {'00443': ['', 443]}, '192.229.210.163': {'00443': ['', 443]}, '192.229.221.012': {'00443': ['', 443]}, '208.085.042.032': {'00080': ['', 80]}, '208.111.190.109': {'00443': ['', 443]}, '2606:2800:21f:dc2:1fe1:23fc:954:1461': {'00443': ['', 443]}, '2606:4700::6811:164b': {'00081': ['', 81], '01194': ['', 1194], '06881': ['', 6881], '08443': ['', 8443], '05061': ['', 5061], '00465': ['', 465], '00995': ['', 995], '08080': ['', 8080], '00443': ['', 443], '00080': ['', 80], '00993': ['', 993], '00853': ['', 853], '01701': ['', 1701]}}, 'udp': {'010.110.049.082': {'63308': ['', 63308]}, '010.110.063.089': {'49882': ['', 49882]}, '010.110.089.150': {'62065': ['', 62065]}, '023.089.015.050': {'05004': ['', 5004]}, '052.112.077.144': {'03480': ['', 3480]}, '054.215.072.028': {'08801': ['', 8801]}, '066.022.214.035': {'50002': ['', 50002]}, '104.044.195.124': {'03478': ['', 3478]}, '142.250.082.217': {'03478': ['', 3478]}, '144.195.033.064': {'08801': ['', 8801]}, '157.240.245.008': {'00443': ['', 443]}, '157.240.245.062': {'03478': ['', 3478]}, '170.133.130.181': {'09000': ['', 9000]}, '2001:4860:4864:5::111': {'19305': ['', 19305]}}}"
)

var (
    // List of UDP replays
    udpSenderCount = []string{"DiscordRandom-06052024", "Discord-06052024", "FacebookVideoRandom-06052024", "FacebookVideo-06052024", "GoogleMeetRandom-04282020", "GoogleMeetRandom-05062024", "GoogleMeet-04282020", "GoogleMeet-05062024", "MicrosoftTeamRandom-04282020", "MicrosoftTeamRandom-05152024", "MicrosoftTeam-04282020", "MicrosoftTeam-05152024", "SkypeRandom-06172024", "SkypeRandom-12122018", "Skype-06172024", "Skype-12122018", "WebexRandom-04282020", "WebexRandom-05152024", "Webex-04282020", "Webex-05152024", "WhatsAppRandom-04112019", "WhatsAppRandom-06072024", "WhatsApp-04112019", "WhatsApp-06072024", "ZoomRandom-04282020", "ZoomRandom-05062024", "Zoom-04282020", "Zoom-05062024"}
)

// Main function for handling old side channel connections.
// clt: client object containing all the information about the test that is running
// first4Bytes: the first 4 bytes of the declare ID data length, which was read to determine that
//     the client uses the old protocol
// Returns any errors
func (sideChannel SideChannel) handleOldSideChannel(conn net.Conn, first4Bytes []byte) error {
    clt, err := sideChannel.oldDeclareID(conn, first4Bytes)
    if err != nil {
        return err
    }
    defer clt.CleanUp(sideChannel.ConnectedClients)

    // if this is the second or subsequent replay, a client object should already exist; use that
    // object instead of the one passed into this function
    client, exists := unanalyzedTests.getClient(clt.UserID, strconv.Itoa(clt.TestID))
    if exists {
        client.Conn = clt.Conn
        currentReplay, err := clt.GetCurrentReplay()
        if err != nil {
            return err
        }
        client.AddReplay(currentReplay.ReplayID, currentReplay.ReplayName, clt.IsLastReplay)
        clt = client
    } else {
        unanalyzedTests.addClient(clt)
    }

    // Receive server side changes (no longer used)
    _, err = sideChannel.oldReadRequest(clt.Conn)
    if err != nil {
        return err
    }

    // Ask 4 permission
    err = sideChannel.oldAsk4Permission(clt)
    if err != nil {
        return err
    }

    // Receive iperf
    err = sideChannel.oldReceiveIperf(clt.Conn)
    if err != nil {
        return err
    }

    // Receive mobile stats
    err = sideChannel.oldReceiveMobileStats(clt)
    if err != nil {
        return err
    }

    // start tcp dump

    // Send server mapping
    err = sideChannel.oldSendResponse(clt.Conn, serverMapping)
    if err != nil {
        return err
    }

    // Send senderCount
    err = sideChannel.oldSendUDPSenderCount(clt)
    if err != nil {
        return err
    }

    // Receive DONE
    replayDuration, err := sideChannel.oldReceiveDone(clt.Conn)
    if err != nil {
        return err
    }

    // Receive throughput info
    err = sideChannel.oldReceiveThroughputs(clt, replayDuration)
    if err != nil {
        return err
    }

    // Send OK
    err = sideChannel.oldSendResponse(clt.Conn, "OK")
    if err != nil {
        return err
    }

    // Receive Result;No
    _, err = sideChannel.oldReadRequest(clt.Conn)
    if err != nil {
        return err
    }

    // stop tcp dump

    // Analysis
    if clt.IsLastReplay {
        err = clt.AnalyzeTest()
        if err != nil {
            return err
        }
    }

    return nil
}

// Receives the ID declared by the clienthandler.
// conn: the connection to the client
// first4Bytes: the first 4 bytes of the declare ID data length, which was read to determine that
//     the client uses the old protocol
// Returns a information about the client or any errors
func (sideChannel SideChannel) oldDeclareID(conn net.Conn, first4Bytes []byte) (*clienthandler.Client, error) {
    // read in the remaining 6 bytes of the 10 byte message length
    dataLengthBytes := make([]byte, 6)
    _, err := io.ReadFull(conn, dataLengthBytes)
    if err != nil {
        return nil, err
    }
    dataLength, err := strconv.Atoi(string(append(first4Bytes, dataLengthBytes...)))
    if err != nil {
        return nil, err
    }

    fmt.Printf("We should read %d bytes %v %d\n", dataLength, dataLengthBytes, len(dataLengthBytes))

    // read in the number of bytes specified by the first read
    buffer := make([]byte, dataLength)
    _, err = io.ReadFull(conn, buffer)
    if err != nil {
        return nil, err
    }

    pieces := strings.Split(string(buffer), ";")
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

    // TODO: this should probably be at the source of the connection
    tcpConn, ok := conn.(*net.TCPConn)
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

// Determines if the client can run a replay.
// If permission is granted, send 1;<client_ip>;<samples_per_replay>.
// If permission is denied, send 0;<error_code>. Sometimes the samples per replay is also sent as
// well.
// clt: the client handler that made the request
// Returns any errors
func (sideChannel SideChannel) oldAsk4Permission(clt *clienthandler.Client) error {
    status, info, err := clt.Ask4Permission(sideChannel.ReplayNames, sideChannel.ConnectedClients)
    if err != nil {
        return err
    }

    var permissionSlice []string
    if status == clienthandler.Ask4PermissionOkStatus {
        permissionSlice = []string{"1", sideChannel.IP, info}
    } else {
        permissionSlice = []string{"0", info}
        if info == clienthandler.Ask4PermissionIPInUseMsg {
            permissionSlice = append(permissionSlice, strconv.Itoa(clienthandler.SamplesPerReplay))
        }
    }

    err = sideChannel.oldSendResponse(clt.Conn, strings.Join(permissionSlice, ";"))
    if err != nil {
        return err
    }

    if status != clienthandler.Ask4PermissionOkStatus {
        return fmt.Errorf("Replay permission issue\n")
    }
    return nil
}

// Receive the request to send iperf information. Unused by the old server but is kept in the old
// protocol due to protocol limitations.
// conn: the client connection
// Returns any errors
func (sideChannel SideChannel) oldReceiveIperf(conn net.Conn) error {
    data, err := sideChannel.oldReadRequest(conn)
    if err != nil {
        return err
    }

    iperfStatus := strings.Split(data, ";")[0]
    if iperfStatus == "WillSendIperf" {
        _, err = sideChannel.oldReadRequest(conn)
        if err != nil {
            return err
        }
    }
    return nil
}

// Receive mobile stats from client.
// clt: the client handler that made the request
// Returns any errors
func (sideChannel SideChannel) oldReceiveMobileStats(clt *clienthandler.Client) error {
    data, err := sideChannel.oldReadRequest(clt.Conn)
    if err != nil {
        return err
    }

    mobileStatsStatus := strings.Split(data, ";")[0]
    if mobileStatsStatus == "WillSendMobileStats" {
        mobileStats, err := sideChannel.oldReadRequest(clt.Conn)
        if err != nil {
            return err
        }

        err = clt.ReceiveMobileStats(mobileStats)
        if err != nil {
            return err
        }
    }
    return nil
}

// Sends UDP sender count to client. If the replay is UDP, a "1" is sent. If the replay is TCP, a
// "0" is sent".
// clt: the client handler that made the request
// Returns any errors
func (sideChannel SideChannel) oldSendUDPSenderCount(clt *clienthandler.Client) error {
    currentReplay, err := clt.GetCurrentReplay()
    if err != nil {
        return err
    }

    // if replay is UDP, send "1"
    for _, replayName := range udpSenderCount {
        if replayName == currentReplay.ReplayName {
            return sideChannel.oldSendResponse(clt.Conn, "1")
        }
    }

    // if replay is TCP, send "0"
    return sideChannel.oldSendResponse(clt.Conn, "0")
}

// Receives DONE message and the replay duration from the client in the format
// DONE;<replay_duration>
// conn: the client connection
// Returns the replay duration (in seconds) or any errors
func (sideChannel SideChannel) oldReceiveDone(conn net.Conn) (string, error) {
    data, err := sideChannel.oldReadRequest(conn)
    if err != nil {
        return "", err
    }

    dataPieces := strings.Split(data, ";")
    if len(dataPieces) < 2 {
        return "", fmt.Errorf("Old side channel DONE expected to receive 2 pieces of data; received %d\n", len(dataPieces))
    }

    // dataPieces[1] is the replay duration in seconds
    return dataPieces[1], nil
}

// Receive the replay throughputs and time samples from the client in the format
// [[throughput_samples],[sample_times]], where throughput_samples and sample_times are
// comma-delimited floats.
// clt: the client handler that made the request
// replayDuration: the time it took for the replay to run in seconds
// Returns any errors
func (sideChannel SideChannel) oldReceiveThroughputs(clt *clienthandler.Client, replayDuration string) error {
    throughputsAndSampleTimes, err := sideChannel.oldReadRequest(clt.Conn)
    if err != nil {
        return err
    }

    return clt.ReceiveThroughputs(replayDuration + ";" + throughputsAndSampleTimes, sideChannel.TmpResultsDir)
}

// Receives data from the client. The old protocol receives data with two reads. The first read is
// the length of the message to be received in bytes, formatted to be a string that is ten
// characters long. The second read contains the actual data.
// conn: the client connection
// Returns the message read or any errors
func (sideChannel SideChannel) oldReadRequest(conn net.Conn) (string, error) {
    // read in 10 bytes of data, which contains the message length
    dataLengthBytes := make([]byte, 10)
    _, err := io.ReadFull(conn, dataLengthBytes)
    if err != nil {
        return "", err
    }
    dataLength, err := strconv.Atoi(string(dataLengthBytes))
    if err != nil {
        return "", err
    }

    fmt.Printf("We should read %d bytes %v %d\n", dataLength, dataLengthBytes, len(dataLengthBytes))

    // read in the number of bytes specified by the first read
    buffer := make([]byte, dataLength)
    _, err = io.ReadFull(conn, buffer)
    if err != nil {
        return "", err
    }

    fmt.Printf("Read %d bytes from client: %s\n", len(buffer), string(buffer))
    return string(buffer), nil
}

// Send data to the client. The old protocol sends data in two writes. The first write sends the
// length of the data in bytes as a string. The string is padded to be ten characters. The second
// write sends the actual data.
// conn: the client connection
// message: the message to send to the client
// Returns any errors
func (sideChannel SideChannel) oldSendResponse(conn net.Conn, message string) error {
    fmt.Println("Sending to client:", message)
    messageLengthStr := strconv.Itoa(len(message))
    messageLengthStrPadded := zfill(messageLengthStr, 10)
    _, err := conn.Write([]byte(messageLengthStrPadded))
    if err != nil {
        return err
    }

    fmt.Printf("Sending %s bytes\n", messageLengthStrPadded)
    _, err = conn.Write([]byte(message))
    if err != nil {
        return err
    }
    return nil
}

// Add leading 0s to a string.
// s: the string to pad
// width: the total length of the new string
// Returns a string with width number of characters, padded by leading 0s
func zfill(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width - len(s)) + s
}
