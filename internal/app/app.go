// Provides the main logic for the Wehe server.
package app

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/tls"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "io/ioutil"
    "math/big"
    "net"
    "os"
    "time"

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

    caKeyPassword := os.Getenv("WEHE_KEY_PASSWORD")
    if caKeyPassword == "" {
        return fmt.Errorf("WEHE_KEY_PASSWORD is not set in environment.")
    }

    cert, err := generateServerCert(cfg.HostInfoFilename, cfg.CACertFilename, cfg.CACertPrivKeyFilename, caKeyPassword, cfg.ServerCertFilename, cfg.ServerCertPrivKeyFilename)
    if err != nil {
        return err
    }

    errChan := make(chan error)
    sideChannel, err := network.NewSideChannel("0.0.0.0", replayNames, cfg.UUIDPrefixFile, cfg.TmpResultsDir, cfg.ResultsDir)
    if err != nil {
        return err
    }
    go sideChannel.StartServer(cert, errChan)

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

    go network.StartOldAnalyzerServer(cert, errChan)

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

// Uses Root CA cert to generate a server cert and writes the server cert and key to file.
// hostInfoFilename: file path to JSON file containing the DNS names and IPs of the server
// caCertFilename: file path to a x509 root CA certificate in PEM format
// caCertPrivKeyFilename: file path to password-protected PEM RSA private key for the root CA cert
// caKeyPassword: the password to the root CA private key
// serverCertFilename: file path to where generated server cert should be written to
// serverCertPrivKeyFilename: file path to where generated server private key should be written to
// Returns the server cert or any errors
func generateServerCert(hostInfoFilename string, caCertFilename string, caCertPrivKeyFilename string, caKeyPassword string, serverCertFilename string, serverCertPrivKeyFilename string) (tls.Certificate, error) {
    // get the DNS names and IP addresses of the server
    hostnames, publicIPs, err := getHostInfo(hostInfoFilename)
    if err != nil {
        return tls.Certificate{}, err
    }
    // common name for server cert should be the DNS name; if server does not have DNS name, use IP
    // as the common name
    commonName := publicIPs[0].String()
    if len(hostnames) > 0 {
        commonName = hostnames[0]
    }

    // read in the root CA cert and key
    caCertPEM, err := ioutil.ReadFile(caCertFilename)
    if err != nil {
        return tls.Certificate{}, err
    }
    caKeyPEM, err := ioutil.ReadFile(caCertPrivKeyFilename)
    if err != nil {
        return tls.Certificate{}, err
    }

    // decode and parse the root CA cert
    caCertPEMBlock, _ := pem.Decode(caCertPEM)
    if caCertPEMBlock == nil {
        return tls.Certificate{}, fmt.Errorf("Cannot decode Root CA Cert.\n")
    }
    caCert, err := x509.ParseCertificate(caCertPEMBlock.Bytes)
    if err != nil {
        return tls.Certificate{}, err
    }

    // decode, decrypt, and parse root CA key
    caKeyPEMBlock, _ := pem.Decode(caKeyPEM)
    if caKeyPEMBlock == nil || caKeyPEMBlock.Type != "RSA PRIVATE KEY" {
        return tls.Certificate{}, fmt.Errorf("Cannont decode Root CA Key\n")
    }
    caKeyDer, err := x509.DecryptPEMBlock(caKeyPEMBlock, []byte(caKeyPassword))
    if err != nil {
        return tls.Certificate{}, err
    }
    caKey, err := x509.ParsePKCS1PrivateKey(caKeyDer)
    if err != nil {
        return tls.Certificate{}, err
    }

    // generate the server key
    serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        return tls.Certificate{}, err
    }

    // generate the server cert
    maxSerial := new(big.Int)
    maxSerial = maxSerial.Exp(big.NewInt(2), big.NewInt(160), nil)
    serial, err := rand.Int(rand.Reader, maxSerial)
    if err != nil {
        return tls.Certificate{}, err
    }
    serverCertTemplate := &x509.Certificate{
        Version: 1,
        SerialNumber: serial,
        Subject: pkix.Name{
            CommonName: commonName,
            Organization: []string{"Northeastern"},
            Province: []string{"MA"},
            Country: []string{"US"},
        },
        NotBefore: time.Now(),
        NotAfter: time.Now().Add(500 * 24 * time.Hour),
        IPAddresses: publicIPs,
        DNSNames: hostnames,
    }
    serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverCertTemplate, caCert, &serverKey.PublicKey, caKey)
    if err != nil {
        return tls.Certificate{}, err
    }

    // encode the server cert and key, and write to file
    serverCertPEM := pem.EncodeToMemory(&pem.Block{
        Type: "CERTIFICATE",
        Bytes: serverCertBytes,
    })
    serverKeyPEM := pem.EncodeToMemory(&pem.Block{
        Type: "RSA PRIVATE KEY",
        Bytes: x509.MarshalPKCS1PrivateKey(serverKey),
    })
    err = ioutil.WriteFile(serverCertFilename, serverCertPEM, 0644)
    if err != nil {
        return tls.Certificate{}, err
    }
    err = ioutil.WriteFile(serverCertPrivKeyFilename, serverKeyPEM, 0600)
    if err != nil {
        return tls.Certificate{}, err
    }
    return tls.Certificate{
        Certificate: [][]byte{serverCertBytes},
        PrivateKey: serverKey,
    }, nil
}

// struct containing DNS names and IP addresses of server
type HostInfo struct {
    Hostnames []string `json:"hostnames"`
    IPs []string `json:"ips"`
}

// Get the DNS names and IP addreses of the server from a JSON file. There must be at least one IP
// address.
// hostInfoFilename: file path to the JSON file containing DNS name and IP address info
// Returns a list of DNS names and IP addresses, or any errors
func getHostInfo(hostInfoFilename string) ([]string, []net.IP, error) {
    infoFile, err := os.Open(hostInfoFilename)
    if err != nil {
        return nil, nil, err
    }
    defer infoFile.Close()

    var hostInfo HostInfo
    decoder := json.NewDecoder(infoFile)
    err = decoder.Decode(&hostInfo)
    if err != nil {
        return nil, nil, err
    }

    if len(hostInfo.IPs) == 0 {
        return nil, nil, fmt.Errorf("There must be at least one IP address.")
    }

    var ips []net.IP
    for _, ip := range hostInfo.IPs {
        parsedIP := net.ParseIP(ip)
        if parsedIP == nil {
            return nil, nil, fmt.Errorf("Cannot parse invalid IP:", ip)
        }
        ips = append(ips, parsedIP)
    }
    return hostInfo.Hostnames, ips, nil
}
