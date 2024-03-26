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
    Responses []Response
    ReplayName string
    IsTCP bool
}

// Either a TCPResponseSet or UDPPacket
type Response interface {

}

// The packets that should be sent after receiving <RequestLength> packets from the client
type TCPResponseSet struct {
    RequestLength int // number of bytes that server should receive before sending the packets
    RequestHash string // hash of the bytes received from client
    Packets []TCPPacket // packets to send to client once server has received RequestLength packets
}

// A TCP packet to be sent as part of a replay
type TCPPacket struct {
    Timestamp time.Duration // time since the last packet received from the client that this packet should be sent
    Payload []byte // the bytes to send to the server
}

func newTCPPacket(timestamp float64, payload string) (TCPPacket, error) {
    payloadBytes, err := hex.DecodeString(payload)
    if err != nil {
        return TCPPacket{}, err
    }
    return TCPPacket{
        Timestamp: time.Duration(timestamp * float64(time.Second)),
        Payload: payloadBytes,
    }, nil
}

// A UDP packet to be sent as part of a replay
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
    Packets []UDPReplayFilePacket `json:"packets"` // the list of packets that are sent to the client
    ResponseSets []ResponseSet `json:"response_sets"`
}

type ResponseSet struct {
    RequestLength int `json:"request_length"`
    RequestHash string `json:"request_hash"`
    Packets []TCPReplayFilePacket `json:"packets"`
}

type TCPReplayFilePacket struct {
    Timestamp float64 `json:"timestamp"`
    Payload string `json:"payload"`
}

// The structure packets are unpacked into from the json file.
type UDPReplayFilePacket struct {
    CSPair string `json:"c_s_pair"` // the client & server of original packet capture, in the form {client_IP}.{client_port}-{server_IP}.{server_port}
    Timestamp float64 `json:"timestamp"` // number of seconds since the start of the replay when this packet should be sent
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

    var responses []Response
    if replayFileInfo.IsTCP {
        // tcp replays
        for _, responseSet := range replayFileInfo.ResponseSets {
            var packets []TCPPacket
            for _, tcpReplayFilePacket := range responseSet.Packets {
                tcpPacket, err := newTCPPacket(tcpReplayFilePacket.Timestamp, tcpReplayFilePacket.Payload)
                if err != nil {
                    return ReplayInfo{}, err
                }
                packets = append(packets, tcpPacket)
            }
            tcpResponseSet := TCPResponseSet{
                RequestLength: responseSet.RequestLength,
                RequestHash: responseSet.RequestHash,
                Packets: packets,
            }
            responses = append(responses, tcpResponseSet)
        }
    } else {
        // udp replays
        for _, udpReplayFilePacket := range replayFileInfo.Packets {
            udpPacket, err := newUDPPacket(udpReplayFilePacket.CSPair, udpReplayFilePacket.Timestamp, udpReplayFilePacket.Payload, udpReplayFilePacket.End)
            if err != nil {
                return ReplayInfo{}, err
            }
            responses = append(responses, udpPacket)
        }
    }

    return ReplayInfo{
        Responses: responses,
        ReplayName: replayFileInfo.ReplayName,
        IsTCP: replayFileInfo.IsTCP,
    }, nil
}
