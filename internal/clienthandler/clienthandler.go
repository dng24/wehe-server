// Handles the logic for receiving and responding to client requests.
package clienthandler

import (
    "fmt"
    "net"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/shirou/gopsutil/v3/cpu"
    "github.com/shirou/gopsutil/v3/disk"
    "github.com/shirou/gopsutil/v3/mem"
    psutilnet "github.com/shirou/gopsutil/v3/net"
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
    clientIPs map[string]struct{} // set of all currently connected client IPs
    mutex sync.Mutex // prevents multiple goroutines from accessing ClientIPs
}

func NewConnectedClients() *ConnectedClients {
    return &ConnectedClients{
        clientIPs: make(map[string]struct{}),
    }
}

func (connectedClients *ConnectedClients) hasKey(key string) bool {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    _, exists := connectedClients.clientIPs[key]
    return exists
}

func (connectedClients *ConnectedClients) addKey(key string) {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    connectedClients.clientIPs[key] = struct{}{}
}

func (connectedClients *ConnectedClients) deleteKey(key string) {
    connectedClients.mutex.Lock()
    defer connectedClients.mutex.Unlock()
    delete(connectedClients.clientIPs, key)
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
    if connectedClientIPs.hasKey(clt.PublicIP) {
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

    connectedClientIPs.addKey(clt.PublicIP)
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

// Determines if the server has enough resources to run the replay.
// Returns false if cpu > 95% or memory > 95% or disk > 95% or network upload > 2000 Mbps; true
//    otherwise or any errors
func (clt Client) hasResources() (bool, error) {
    // TODO: old server doesn't actually use cpu for check
    cpuUsage, err := cpu.Percent(time.Second, false)
    if err != nil {
        return false, err
    }
    fmt.Println("cpu:", cpuUsage[0])
    if cpuUsage[0] > 95 {
        return false, nil
    }

    memUsage, err := mem.VirtualMemory()
    if err != nil {
        return false, err
    }
    fmt.Println("mem:", memUsage.UsedPercent)
    if memUsage.UsedPercent > 95 {
        return false, nil
    }

    diskUsage, err := disk.Usage("/")
    if err != nil {
        return false, err
    }
    fmt.Println("disk:", diskUsage.UsedPercent)
    if diskUsage.UsedPercent > 95 {
        return false, nil
    }

    netUsage, err := psutilnet.IOCounters(false)
    if err != nil {
        return false, err
    }
    bytesSent0 := netUsage[0].BytesSent
    time.Sleep(1 * time.Second)
    netUsage, err = psutilnet.IOCounters(false)
    if err != nil {
        return false, err
    }
    bytesSent1 := netUsage[0].BytesSent
    uploadMbps := float64((bytesSent1 - bytesSent0) * 8) / 1000000.0
    fmt.Println("net:", uploadMbps)
    if uploadMbps > 2000 {
        return false, err
    }

    return true, nil
}

func (clt Client) ReceiveDeviceInfo(message string) {

}

func (clt Client) CleanUp(connectedClientIPs *ConnectedClients) {
    fmt.Println("Cleaning up connection to", clt.PublicIP)
    connectedClientIPs.deleteKey(clt.PublicIP)
}
