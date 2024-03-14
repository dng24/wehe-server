// Parses and provides the server replays.
package testdata

import (
    "encoding/hex"
    "encoding/json"
    "os"
    "path/filepath"
    "time"
)

const (
    replaysRoot = "res/replays/"
)

type ReplayInfo struct {
    Packets []Packet
    ReplayName string
    IsTCP bool
}

// Either a TCPPacket or UDPPacket
type Packet interface {

}

// A UDP packet to be sent as part of a replay.
type UDPPacket struct {
    CSPair string // the client & server of original packet capture, in the form {client_IP}.{client_port}-{server_IP}.{server_port}
    Timestamp time.Duration // time since the start of the replay when this packet should be sent
    Payload []byte // the bytes to send to the server
    End bool // ???
}

func newUDPPacket(csPair string, timestamp float64, payload string, end bool) (UDPPacket, error) {
    payloadBytes, err := hex.DecodeString(payload)
    if err != nil {
        return UDPPacket{}, err
    }
    return UDPPacket{
        CSPair: csPair,
        Timestamp: time.Duration(timestamp * float64(time.Second)),
        Payload: payloadBytes,
        End: end,
    }, nil
}

// The structure that replay files get unpacked into.
type ReplayFileInfo struct {
    ReplayName string `json:"test_name"` // name of the replay
    IsTCP bool `json:"is_tcp"` // true if replay is TCP, false if replay is UDP
    Packets []ReplayFilePacket `json:"packets"` // the list of packets that are sent to the client
}

// The structure packets are unpacked into from the json file.
type ReplayFilePacket struct {
    CSPair string `json:"c_s_pair"` // the client & server of original packet capture, in the form {client_IP}.{client_port}-{server_IP}.{server_port}
    Timestamp float64 `json:"timestamp"` // time since the start of the replay when this packet should be sent
    Payload string `json:"payload"`// the bytes to send to the server
    End bool `json:"end"` // ???
}

// Loads the tests from disk.
// replayName: the name of the replay to load.
// Returns information about the replay along with the list of packets to send to the client, or any errors
func ParseReplayJSON(replayName string) (ReplayInfo, error) {
    // get the filepath, which is replayRootFolder/replayName/replayName.pcap_server_all.json
    replayFile := filepath.Join(replaysRoot, replayName, replayName + ".pcap_server_all.json")
    // read in the file
    data, err := os.ReadFile(replayFile)
    if err != nil {
        return ReplayInfo{}, err
    }

    // unpack as json object
    var replayFileInfo ReplayFileInfo
    err = json.Unmarshal(data, &replayFileInfo)
    if err != nil {
        return ReplayInfo{}, err
    }

    var packets []Packet
    if replayFileInfo.IsTCP {
        // tcp replays
    } else {
        // udp replays
        for _, replayFilePacket := range replayFileInfo.Packets {
            udpPacket, err := newUDPPacket(replayFilePacket.CSPair, replayFilePacket.Timestamp, replayFilePacket.Payload, replayFilePacket.End)
            if err != nil {
                return ReplayInfo{}, err
            }
            packets = append(packets, udpPacket)
        }
    }

    return ReplayInfo{
        Packets: packets,
        ReplayName: replayFileInfo.ReplayName,
        IsTCP: replayFileInfo.IsTCP,
    }, nil
}
