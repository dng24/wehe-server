// Represents a Wehe client connected to the server
package client

import (
    "net"
    "strconv"
    "strings"
)

//TODO: move to replay file when that exists
type ReplayType int

const (
    Original ReplayType = iota
    Random
)

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

func (clt Client) Ask4Permission() {

}

func (clt Client) ReceiveDeviceInfo(message string) {

}
