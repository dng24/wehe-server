// Handles the logic for receiving and responding to client requests.
// TODO: implement timeout for client so that connection doesn't keep running forever in the event that client crashes
package clienthandler

import (
    "encoding/json"
    "fmt"
    "math"
    "net"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/shirou/gopsutil/v3/disk"
    "github.com/shirou/gopsutil/v3/mem"
    psutilnet "github.com/shirou/gopsutil/v3/net"

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

type Client struct {
    Conn net.Conn // the connection to the client
    UserID string // the 10 character user ID
    ReplayID ReplayType // indicates whether replay is the original or random replay
    ReplayName string // name of the replay to run
    ExtraString string // extra information; in the current version, it is number attempts client makes to MLab before successful connection
    TestID int // the ID of the test for the particular user
    IsLastReplay bool // true if this is the last replay of the test; false otherwise
    PublicIP string // public IP of the client retrieved from the test port
    ClientVersion string // client version number of Wehe
    StartTime time.Time // time when side channel connection was made
}

func (clt Client) GetMajorVersionNumber() (int, error) {
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
//    is returned as the info; if status is failure, then failure code is returned as the info
func (clt Client) Ask4Permission(replayNames []string, connectedClientIPs *ConnectedClients) (string, string) {
    // Client can't run replay if replay is not on the server
    if !clt.replayExists(replayNames) {
        return ask4PermissionErrorStatus, ask4PermissionUnknownReplayMsg
    }

    // We allow only one client per IP at a time because multiple clients on an IP might affect throughputs
    if connectedClientIPs.Has(clt.PublicIP) {
        return ask4PermissionErrorStatus, ask4PermissionIPInUseMsg
    }

    // Don't run replays if server is overloaded (>95% CPU, mem, disk, or >2000 Mbps network)
    hasResources, err := clt.hasResources()
    if err != nil {
        return ask4PermissionErrorStatus, ask4PermissionResourceRetrievalFailMsg
    }
    if !hasResources {
        return ask4PermissionErrorStatus, ask4PermissionLowResourcesMsg
    }
    connectedClientIPs.add(clt.PublicIP, clt.ReplayName)
    return ask4PermissionOkStatus, strconv.Itoa(samplesPerReplay)
}

// Checks if the replay that client would like to run is present on server.
// replayNames: list of all the replay names on the server
// Returns true if server has replay client wants to run; false otherwise
func (clt Client) replayExists(replayNames []string) bool {
    for _, replayName := range replayNames {
        if replayName == clt.ReplayName {
            return true
        }
    }
    return false
}

// Determines if the server has enough resources to run the replay. Don't deny permission if
// resources can't be retrieved.
// Returns false if memory > 95% or disk > 95% or network upload > 2000 Mbps; true
//    otherwise or any errors
func (clt Client) hasResources() (bool, error) {
    memUsage, err := mem.VirtualMemory()
    if err == nil {
        fmt.Println("mem:", memUsage.UsedPercent)
        if memUsage.UsedPercent > 95 {
            return false, nil
        }
    }

    diskUsage, err := disk.Usage("/")
    if err == nil {
        fmt.Println("disk:", diskUsage.UsedPercent)
        if diskUsage.UsedPercent > 95 {
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
                return false, nil
            }
        }
    }

    return true, nil
}

// Receives information about the client mobile device, network, and location.
// message: json containing the device, network, and location information
// Returns any errors
func (clt Client) ReceiveMobileStats(message string) error {
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
    fmt.Printf("mobile stats: %v", mobileStatsData)
    //TODO: figure out what to do with mobile stats once it is processed
    return nil
}

func (clt Client) CleanUp(connectedClientIPs *ConnectedClients) {
    fmt.Println("Cleaning up connection to", clt.PublicIP)
    connectedClientIPs.del(clt.PublicIP)
}
