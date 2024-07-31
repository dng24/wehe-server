// Handles the logic for receiving and responding to client requests.
// TODO: implement timeout for client so that connection doesn't keep running forever in the event that client crashes
package clienthandler

import (
    "encoding/json"
    "fmt"
    "math"
    "net"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/shirou/gopsutil/v3/disk"
    "github.com/shirou/gopsutil/v3/mem"
    psutilnet "github.com/shirou/gopsutil/v3/net"

    "wehe-server/internal/analysis"
    "wehe-server/internal/geolocation"
)

const (
    samplesPerReplay = 100 //TODO: think ab if this should be in config file - theoretically, all clients should work if this changes
    ask4PermissionOkStatus = "0"
    ask4PermissionErrorStatus = "1"
    ask4PermissionUnknownReplayMsg = "1"
    ask4PermissionIPInUseMsg = "2"
    ask4PermissionLowResourcesMsg = "3"
    ask4PermissionResourceRetrievalFailMsg = "4"
)

//TODO: move to replay file when that exists
type ReplayType int

const (
    Original ReplayType = iota
    Random
)

type ConnectedClients struct {
    clientIPs map[string]string // map of all currently connected client IPs to the replay they want to run
    mutex sync.Mutex // prevents multiple goroutines from accessing ClientIPs
}

func NewConnectedClients() *ConnectedClients {
    return &ConnectedClients{
        clientIPs: make(map[string]string),
    }
}

// Checks if client is currently running a replay.
// ip: IP of the client
// Returns true if client is running a replay; false otherwise
func (connectedClients *ConnectedClients) Has(ip string) bool {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    _, exists := connectedClients.clientIPs[ip]
    return exists
}

// Gets the replay name that a connected client is currently running.
// ip: IP of the client
// Returns the replay name of the client with the given IP or any errors
func (connectedClients *ConnectedClients) Get(ip string) (string, error) {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    replayName, exists := connectedClients.clientIPs[ip]
    if exists {
        return replayName, nil
    } else {
        return "", fmt.Errorf("%s is not currently running a replay.\n", ip)
    }
}

// Adds a client with it starts a replay.
// ip: the IP of the client
// replayName: the name of the replay that the client would like to run
func (connectedClients *ConnectedClients) add(ip string, replayName string) {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    connectedClients.clientIPs[ip] = replayName
}

// Removes a client.
// ip: the IP of the client to remove
func (connectedClients *ConnectedClients) del(ip string) {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    delete(connectedClients.clientIPs, ip)
}

// Information about the data generated from a replay.
type ReplayResult struct {
    ReplayID ReplayType // indicates whether replay is the original or random replay
    ReplayName string // name of the replay to run
    Throughputs []float64 // throughput samples
    SampleTimes []float64 // list of the number of seconds since start of replay that each throughput sample was captured
    ReplayDuration time.Duration // time it took to run the replay
}

// Information about a client. Each test gets a Client struct.
type Client struct {
    Conn net.Conn // the connection to the client
    UserID string // the 10 character user ID
    ExtraString string // extra information; in the current version, it is number attempts client makes to MLab before successful connection
    TestID int // the ID of the test for the particular user
    IsLastReplay bool // true if this is the last replay of the test; false otherwise
    PublicIP string // public IP of the client retrieved from the test port
    ClientVersion string // client version number of Wehe
    MobileStats map[string]interface{} // information about the client device
    StartTime time.Time // time when side channel connection was made
    Exceptions string // any errors that occurred while running a replay
    MLabUUID string // globally unique ID for M-Lab
    ReplayResults []ReplayResult // data collected from running a replay
    Analysis *analysis.AnalysisResults // analysis results of the test
}

// Constructs a new Client.
// conn: the side channel connection to the client
// userID: the 10-character user ID that identifies a device
// extraString: extra information
// testID: identifies the test for the given user
// publicIP: public IP of the client retrieved from the test port
// clientVersion: version number of the Wehe client
// mlabUUID: globally unique ID for M-Lab
// Returns a pointer to a Client
func NewClient(conn net.Conn, userID string, extraString string, testID int, publicIP string, clientVersion string, mlabUUID string) *Client {
    return &Client{
        Conn: conn,
        UserID: userID,
        ExtraString: extraString,
        TestID: testID,
        PublicIP: publicIP,
        ClientVersion: clientVersion,
        StartTime: time.Now().UTC(),
        Exceptions: "NoExp",
        MLabUUID: mlabUUID,
        ReplayResults: []ReplayResult{},
    }
}

// Adds a replay to the Client. A replay must be added to the Client before it can begin.
// replayID: the type of replay to run
// replayName: the name of the replay
// isLastReplay: true if this replay is the last replay in the test; false otherwise
func (clt *Client) AddReplay(replayID ReplayType, replayName string, isLastReplay bool) {
    replayResult := ReplayResult{
        ReplayID: replayID,
        ReplayName: replayName,
    }
    clt.ReplayResults = append(clt.ReplayResults, replayResult)
    clt.IsLastReplay = isLastReplay
}

// Retrieves the replay that was last added.
// Returns the replay last added, or any errors
func (clt *Client) getCurrentReplay() (*ReplayResult, error) {
    replayResultsLen := len(clt.ReplayResults)
    if replayResultsLen == 0 {
        return &ReplayResult{}, fmt.Errorf("There are currently no replays.\n")
    }
    return &clt.ReplayResults[len(clt.ReplayResults) - 1], nil
}

func (clt *Client) GetMajorVersionNumber() (int, error) {
    num, err := strconv.Atoi(strings.Split(clt.ClientVersion, ".")[0])
	if err != nil {
		return -1, err
	}
    return num, nil
}

//TODO: look at https://github.com/NEU-SNS/wehe-py3/blob/master/src/replay_server.py#L809 again -- why is ask4permission >120 lines ??? also killIfNeeded(), admissionCtrl, inProgress, id vs realID ???
// Determines if client can run a replay.
// replayNames: names of all replays
// connectedClientIPs: all the client IPs that are currently connected to the server
// Returns a status code and information; if status is success, then number of samples per replay
//    is returned as the info; if status is failure, then failure code is returned as the info;
//    or any errors
func (clt *Client) Ask4Permission(replayNames []string, connectedClientIPs *ConnectedClients) (string, string, error) {
    currentReplay, err := clt.getCurrentReplay()
    if err != nil {
        return "", "", err
    }

    // Client can't run replay if replay is not on the server
    if !clt.replayExists(replayNames, currentReplay.ReplayName) {
        clt.Exceptions = "UnknownRelplayName"
        return ask4PermissionErrorStatus, ask4PermissionUnknownReplayMsg, nil
    }

    // We allow only one client per IP at a time because multiple clients on an IP might affect throughputs
    if connectedClientIPs.Has(clt.PublicIP) {
        clt.Exceptions = "NoPermission"
        return ask4PermissionErrorStatus, ask4PermissionIPInUseMsg, nil
    }

    // Don't run replays if server is overloaded (>95% CPU, mem, disk, or >2000 Mbps network)
    hasResources, err := clt.hasResources(len(connectedClientIPs.clientIPs))
    if err != nil {
        return ask4PermissionErrorStatus, ask4PermissionResourceRetrievalFailMsg, nil
    }
    if !hasResources {
        return ask4PermissionErrorStatus, ask4PermissionLowResourcesMsg, nil
    }

    connectedClientIPs.add(clt.PublicIP, currentReplay.ReplayName)
    return ask4PermissionOkStatus, strconv.Itoa(samplesPerReplay), nil
}

// Checks if the replay that client would like to run is present on server.
// replayNames: list of all the replay names on the server
// currentReplayName: the name of the replay to check if it exists
// Returns true if server has replay client wants to run; false otherwise
func (clt *Client) replayExists(replayNames []string, currentReplayName string) bool {
    for _, replayName := range replayNames {
        if replayName == currentReplayName {
            return true
        }
    }
    return false
}

// Determines if the server has enough resources to run the replay. Don't deny permission if
// resources can't be retrieved.
// numConnectedClients: the number of clients currently connected to the server
// Returns false if memory > 95% or disk > 95% or network upload > 2000 Mbps; true
//    otherwise or any errors
func (clt *Client) hasResources(numConnectedClients int) (bool, error) {
    memUsage, err := mem.VirtualMemory()
    if err == nil {
        fmt.Println("mem:", memUsage.UsedPercent)
        if memUsage.UsedPercent > 95 {
            clt.Exceptions = fmt.Sprintf("Server Overloaded with Memory Usage %d%% with %d active connections now ***", memUsage.UsedPercent, numConnectedClients)
            return false, nil
        }
    }

    diskUsage, err := disk.Usage("/")
    if err == nil {
        fmt.Println("disk:", diskUsage.UsedPercent)
        if diskUsage.UsedPercent > 95 {
            clt.Exceptions = fmt.Sprintf("Server Overloaded with Disk Usage %d%% with %d active connections now ***", diskUsage.UsedPercent, numConnectedClients)
            return false, nil
        }
    }

    netUsage, err := psutilnet.IOCounters(false)
    if err == nil {
        bytesSent0 := netUsage[0].BytesSent
        time.Sleep(1 * time.Second)
        netUsage, err = psutilnet.IOCounters(false)
        if err == nil {
            bytesSent1 := netUsage[0].BytesSent
            uploadMbps := float64((bytesSent1 - bytesSent0) * 8) / 1000000.0
            fmt.Println("net:", uploadMbps)
            if uploadMbps > 2000 {
                clt.Exceptions = fmt.Sprintf("Server Overloaded with Upload Bandwidth Usage %dMbps with %d active connections now ***", uploadMbps, numConnectedClients)
                return false, nil
            }
        }
    }

    return true, nil
}

// Receives information about the client mobile device, network, and location.
// message: json containing the device, network, and location information
// Returns any errors
func (clt *Client) ReceiveMobileStats(message string) error {
    fmt.Println("MOBILE STATS", message)
    var mobileStatsData map[string]interface{}
    err := json.Unmarshal([]byte(message), &mobileStatsData)
    if err != nil {
        return err
    }

    locationInfo, ok := mobileStatsData["locationInfo"].(map[string]interface{})
    if !ok {
        return fmt.Errorf("No 'locationInfo' key in mobile stats JSON, or value is not a dictionary.")
    }
    latStr, ok := locationInfo["latitude"].(string)
    if !ok {
        return fmt.Errorf("No 'latitude' key in mobile stats JSON, or value is not a string.")
    }
    longStr, ok := locationInfo["longitude"].(string)
    if !ok {
        return fmt.Errorf("No 'longitude' key in mobile stats JSON, or value is not a string.")
    }
    // if location is given, do reverse geocode lookup and get local time
    if latStr != "nil" && longStr != "nil" && latStr != "0.0" && longStr != "0.0" {
        lat, err := strconv.ParseFloat(latStr, 64)
        if err != nil {
            return err
        }
        long, err := strconv.ParseFloat(longStr, 64)
        if err != nil {
            return err
        }
        lat = math.Round(lat * 10) / 10
        long = math.Round(long * 10) / 10
        // get city and country of client
        loc, err := geolocation.ReverseGeocode(lat, long)
        if err != nil {
            return err
        }
        timeZoneLocation, err := time.LoadLocation(loc.TimeZone)
        if err != nil {
            return err
        }
        locationInfo["country"] = loc.Country
        locationInfo["city"] = loc.City
        locationInfo["localTime"] = clt.StartTime.In(timeZoneLocation).Format("2006-01-02 15:04:05-0700")
        locationInfo["latitude"] = lat
        locationInfo["longitude"] = long
    }
    clt.MobileStats = mobileStatsData
    fmt.Printf("mobile stats: %v", mobileStatsData)
    return nil
}

// Receives the duration of the replay, throughputs, and the sample times after a replay has been
// run. Writes throughputs to tempResultsDir/userID/clientXputs/Xput_<userID>_<testID>_<replayID>.json.
// message: the data that has been received from the client
// resultsDir: the root directory of the results to place the throughputs in
// Returns any errors
func (clt *Client) ReceiveThroughputs(message string, resultsDir string) error {
    currentReplay, err := clt.getCurrentReplay()
    if err != nil {
        return err
    }

    // format: <replayDuration>;<[[throughputs],[sampleTimes]]
    data := strings.Split(message, ";")
    if len(data) < 2 {
        return fmt.Errorf("Received improperly formatted throughput data: %s\n", message)
    }
    replayDurationFloat, err := strconv.ParseFloat(data[0], 64)
    if err != nil {
        return err
    }
    currentReplay.ReplayDuration = time.Duration(replayDurationFloat * float64(time.Second))

    var throughputsAndSampleTimes [][]float64
    err = json.Unmarshal([]byte(data[1]), &throughputsAndSampleTimes)
    if err != nil {
        return err
    }

    if len(throughputsAndSampleTimes) != 2 {
        return fmt.Errorf("Received improperly formatted throughput and sample times. 2 items expected, received %d\n", len(throughputsAndSampleTimes))
    }
    currentReplay.Throughputs = throughputsAndSampleTimes[0]
    currentReplay.SampleTimes = throughputsAndSampleTimes[1]

    // write the throughputs and sample times to file; TODO: move to file writing function
    throughputDir := filepath.Join(resultsDir, clt.UserID, "clientXputs")
    filename := "Xput_" + clt.UserID + "_" + strconv.Itoa(clt.TestID) + "_" + strconv.Itoa(int(currentReplay.ReplayID)) + ".json"

    err = writeToFile(throughputDir, filename, data[1])
    if err != nil {
        return err
    }
    return nil
}

// Receives a request to run additional replays in a test. Request to run the first replay in a
// test is sent in DeclareID. Replay is checked if it exists on server.
// replayNames: the names of all replays available to run
// message: the data that has been received from the client
// Returns a status code and information; if status is success, then number of samples per replay
//    is returned as the info; if status is failure, then failure code is returned as the info;
//    and any errors
func (clt *Client) DeclareReplay(replayNames []string, message string) (string, string, error) {
    // message is <replayID>;<replayName>;<isLastReplay>
    pieces := strings.Split(message, ";")
    if len(pieces) < 3 {
        return "", "", fmt.Errorf("Expected to receive at least 3 pieces from declare replay; only received %d.\n", len(pieces))
    }

    replayIDInt, err := strconv.Atoi(pieces[0])
    if err != nil {
        return "", "", err
    }
    var replayID ReplayType
    if replayIDInt == 0 {
        replayID = Original
    } else if replayIDInt == 1 {
        replayID = Random
    } else {
        return "", "", fmt.Errorf("Unexpected replay ID: %d; must be 0 (original) or 1 (random)", replayIDInt)
    }

    //TODO: change client replay files replay names to use _ instead of -, then delete this terrible replace code
    replayName := strings.Replace(pieces[1], "-", "_", -1)

    isLastReplay, err := strToBool(pieces[2])
    if err != nil {
        return "", "", err
    }

    clt.AddReplay(replayID, replayName, isLastReplay)

    // Client can't run replay if replay is not on the server
    if !clt.replayExists(replayNames, replayName) {
        clt.Exceptions = "UnknownRelplayName"
        return ask4PermissionErrorStatus, ask4PermissionUnknownReplayMsg, nil
    }

    return ask4PermissionOkStatus, strconv.Itoa(samplesPerReplay), nil
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

// Analyzes the test by performing a 2 sample KS test on the throughputs of the original and random
// replays.
// Returns any errors
func (clt *Client) AnalyzeTest() error {
    //TODO: rename all AnalyzeTest to 2 sample KS test
    if len(clt.ReplayResults) < 2 {
        return fmt.Errorf("There needs to be two results to do 2-sample KS test. There are currently %d results.\n", len(clt.ReplayResults))
    }

    // determine order of replay types
    var originalReplayIndex int
    var randomReplayIndex int
    if clt.ReplayResults[0].ReplayID == Original && clt.ReplayResults[1].ReplayID == Random {
        originalReplayIndex = 0
        randomReplayIndex = 1
    } else if clt.ReplayResults[0].ReplayID == Random && clt.ReplayResults[1].ReplayID == Original {
        randomReplayIndex = 0
        originalReplayIndex = 1
    } else {
        return fmt.Errorf("Invalid replay types for 2-sample KS test: %v and %v\n", clt.ReplayResults[0].ReplayID, clt.ReplayResults[1].ReplayID)
    }

    // do analyses
    originalReplayStats, err := analysis.NewDataSetStats(clt.ReplayResults[originalReplayIndex].Throughputs)
    if err != nil {
        return err
    }
    randomReplayStats, err := analysis.NewDataSetStats(clt.ReplayResults[randomReplayIndex].SampleTimes)
    if err != nil {
        return err
    }
    area := randomReplayStats.Average - originalReplayStats.Average

    xputMin := analysis.CalculateMinValueOfTwoSlices(originalReplayStats.Data, randomReplayStats.Data)
    areaOvar := analysis.CalculateArea0Var(originalReplayStats.Average, randomReplayStats.Average)
    ks2dVal, ks2pVal, err := analysis.KS2Samp(originalReplayStats.Data, randomReplayStats.Data)
    if err != nil {
        return err
    }
    dValAvg, pValAvg, ks2AcceptRatio, err := analysis.SampleKS2(originalReplayStats.Data, randomReplayStats.Data, ks2pVal)
    if err != nil {
        return err
    }
    clt.Analysis = analysis.NewAnalysisResults(originalReplayStats, randomReplayStats, area, xputMin,
        areaOvar, ks2dVal, ks2pVal, dValAvg, pValAvg, ks2AcceptRatio)

    //TODO: write to file
    fmt.Println("Analysis results:", clt.Analysis)
    return nil
}

// Anonymizes an IP address by returning the /24 of an IPv4 address or /48 of an IPv6 address.
// ipString: the IP address to anonyize
// Returns the anonyimzed IP address or any errors
func getAnonIP(ipString string) (string, error) {
    ip := net.ParseIP(ipString)
    if ip == nil {
        return "", fmt.Errorf("%s is not a valid IP address.\n", ipString)
    }

    ipv4 := ip.To4()
    if ipv4 != nil {
        mask := net.CIDRMask(24, 32) // /24 mask
        anonIP := ipv4.Mask(mask)
        return anonIP.String(), nil
    }

    ipv6 := ip.To16()
    if ipv6 != nil {
        mask := net.CIDRMask(48, 128) // /48 mask
        anonIP := ipv6.Mask(mask)
        return anonIP.String(), nil
    }

    return "", fmt.Errorf("Unknown IP address type: %s\n", ipString)
}

// Writes information about the replay to disk in a JSON array. The contents of the file match the
// format of the old server; therefore some fields may be obsolete. Writes information to
// tempResultsDir/userID/replayInfo/replayInfo_<userID>_<testID>_<replayID>.json.
//
// Items written to disk include:
// 1. Replay start time - this is the time when the server received the client connection, the
//    format being YYYY-MM-DD HH:MM:SS, in UTC
// 2. User ID
// 3. Anonymized client public IP
// 4. Anonymized client public IP, again
// 5. Name of the replay
// 6. Extra string
// 7. Test ID, as a string
// 8. Replay ID, as a string
// 9. Any exceptions (in practice this is always "NoExp", as if there is an exception, the code
//    does not reach this point, even in the old version of the server)
// 10. Whether the replay packets finish sending, as a boolean (this is always true, as the code,
//     even in the old version, does not reach this point if the packets do not successfully send)
// 11. Whether "result;no" and jitter are sent successfully, as a boolean (this is deprecated, so
//     true is always sent)
// 12. The iperf rate (this is deprecated, so it is always nil)
// 13. The elapsed time, in seconds, between the client connection start time (#1) and now, as a float
// 14. The number of seconds it took for the client to send its packets, as a string
// 15. The mobile stats, as an escaped string in JSON format
// 16. The boolean false
// 17. Version number of the Wehe client
// 18. A M-Lab globally unique UUID
//
// resultsDir: the root directory of the results to place the replay information in
// Returns any errors
func (clt *Client) WriteReplayInfoToFile(resultsDir string) error {
    currentReplay, err := clt.getCurrentReplay()
    if err != nil {
        return err
    }

    // convert start time into proper format
    startTimeFormatted := clt.StartTime.Format("2006-01-02 15:04:05")
    anonIP, err := getAnonIP(clt.PublicIP)
    if err != nil {
        return err
    }

    // convert mobile stats into a string
    mobileStatsString, err := json.Marshal(clt.MobileStats)
    if err != nil {
        return err
    }

    // form the output JSON
    outputItems := []interface{}{
        startTimeFormatted, // 1
        clt.UserID, // 2
        anonIP, // 3
        anonIP, // 4
        currentReplay.ReplayName, // 5
        clt.ExtraString, // 6
        strconv.Itoa(clt.TestID), // 7
        strconv.Itoa(int(currentReplay.ReplayID)), // 8
        clt.Exceptions, // 9
        true, // 10
        true, // 11
        nil, // 12
        time.Since(clt.StartTime).Seconds(), // 13
        strconv.FormatFloat(currentReplay.ReplayDuration.Seconds(), 'f', 9, 64), // 14
        string(mobileStatsString), // 15
        false, // 16
        clt.ClientVersion, // 17
        clt.MLabUUID, // 18
    }
    jsonArrayOutput, err := json.Marshal(outputItems)
    if err != nil {
        return err
    }

    // write replay information to disk
    replayInfoDir := filepath.Join(resultsDir, clt.UserID, "replayInfo")
    filename := "replayInfo_" + clt.UserID + "_" + strconv.Itoa(clt.TestID) + "_" + strconv.Itoa(int(currentReplay.ReplayID)) + ".json"
    err = writeToFile(replayInfoDir, filename, string(jsonArrayOutput))
    if err != nil {
        return err
    }
    return nil
}

func (clt *Client) CleanUp(connectedClientIPs *ConnectedClients) {
    fmt.Println("Cleaning up connection to", clt.PublicIP)
    connectedClientIPs.del(clt.PublicIP)
}

// Write contents to a file. Any missing directories will be created.
// parentDir: the parent directory of the file
// filename: the name of the file
// contents: the contents of the file to write
func writeToFile(parentDir string, filename string, contents string) error {
    if err := os.MkdirAll(parentDir, 0755); err != nil {
        return err
    }

    file, err := os.Create(filepath.Join(parentDir, filename))
    if err != nil {
        return err
    }
    defer file.Close()

    if _, err := file.WriteString(contents); err != nil {
        return err
    }
    return nil
}
