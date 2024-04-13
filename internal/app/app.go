// Provides the main logic for the Wehe server.
package app

import (
    "encoding/json"
    "fmt"
    "os"

    "wehe-server/internal/config"
    "wehe-server/internal/geolocation"
    "wehe-server/internal/network"
)

type TestPortNumbers struct {
    TCPPorts []int `json:"tcp_ports"`
    UDPPorts []int `json:"udp_ports"`
}

// Run the Wehe server.
// cfg: the configurations to run Wehe with
// Returns any errors
func Run(cfg config.Config) error {
    replayNames, err := getReplayNames(cfg.TestsDir)
    if err != nil {
        return err
    }
    portNumbers, err := getTestPorts(cfg.PortNumbersFile)
    if err != nil {
        return err
    }

    err = geolocation.Init()
    if err != nil {
        return err
    }

    errChan := make(chan error)
    sideChannel := network.NewSideChannel("0.0.0.0", replayNames, cfg.ResultsDir)
    go sideChannel.StartServer(errChan)

    // TODO: revisit this comment - will we still use WHATSMYIPMAN? will it be on a separate port?
    // for backwards compatibility, we open all TCP and UDP replay ports needed to run all tests
    // during server initialization since clients v3.7.4 and older will make a request to the test
    // port to get its public IP (WHATSMYIPMAN) before it connects to the side channel, so we don't
    // know when client will make a request to a test port
    var tcpServers []network.TCPServer
    var udpServers []network.UDPServer
    for _, port := range portNumbers.TCPPorts {
        tcpServer := network.NewTCPServer("0.0.0.0", port, sideChannel.ConnectedClients)
        go tcpServer.StartServer(errChan)
        tcpServers = append(tcpServers, tcpServer)
    }

    for _, port := range portNumbers.UDPPorts {
        udpServer := network.NewUDPServer("0.0.0.0", port, sideChannel.ConnectedClients)
        go udpServer.StartServer(errChan)
        udpServers = append(udpServers, udpServer)
    }

    err = <-errChan
    if err != nil {
        return err
    }
    return nil
}

// Get names of the replays, which are used by the client to tell the server which replay it wants
// to use. The replay name is the name of the directory that the replay file is contained in.
// dirPath: the path to a directory containing directories which contain the replay files
// Returns a list of replay names or an error
func getReplayNames(dirPath string) ([]string, error) {
    var testNames []string

    entries, err := os.ReadDir(dirPath)
    if err != nil {
        return nil, err
    }

    for _, entry := range entries {
        if entry.IsDir() {
            testNames = append(testNames, entry.Name())
        }
    }

    return testNames, nil
}

// Get port numbers for all replays.
// portFile: path to a file containing the ports needed to be opened to run all tests
// Returns TCP and UDP port numbers or an error
func getTestPorts(portFile string) (TestPortNumbers, error) {
    data, err := os.ReadFile(portFile)
    if err != nil {
        return TestPortNumbers{}, err
    }

    var testPortNumbers TestPortNumbers
    err = json.Unmarshal(data, &testPortNumbers)
    if err != nil {
        return TestPortNumbers{}, err
    }

    for _, port := range testPortNumbers.TCPPorts {
        if port < 0 || port > 65535 {
            return TestPortNumbers{}, fmt.Errorf("TCP port %d in %s is not a valid port number.", port, portFile)
        }
    }

    for _, port := range testPortNumbers.UDPPorts {
        if port < 0 || port > 65535 {
            return TestPortNumbers{}, fmt.Errorf("UDP port %d in %s is not a valid port number.", port, portFile)
        }
    }

    return testPortNumbers, err
}
