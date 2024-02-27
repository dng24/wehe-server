// Does packet captures.
// TODO: after implementing tests, fix packet capture to tcpdump filters by port, then editcap truncates the payload while preserving everything in the headers, and then tcprewrite updates the client IP/checksum
// https://github.com/NEU-SNS/wehe-py3/blob/master/src/python_lib.py#L653
package network

import (
    "os"
    "path/filepath"

    "github.com/google/gopacket"
    "github.com/google/gopacket/layers"
    "github.com/google/gopacket/pcapgo"
)

type PacketCapture struct {
    iface string // the interface to listen to
    handle *pcapgo.EthernetHandle // the socket to capture packets
    packets []gopacket.Packet // list of packets captured
}

func NewPacketCapture(iface string) (*PacketCapture, error) {
    handle, err := pcapgo.NewEthernetHandle(iface)
    if err != nil {
        return nil, err
    }
    return &PacketCapture{
        iface: iface,
        handle: handle,
    }, nil
}

// Captures packets. This function should be run in a new thread, as it does not return until
// StopPacketCapture is called.
func (packetCapture *PacketCapture) StartPacketCapture() {
    packetSrc := gopacket.NewPacketSource(packetCapture.handle, layers.LayerTypeEthernet)
    // capture packets
    for packet := range packetSrc.Packets() {
        packetCapture.packets = append(packetCapture.packets, packet)
    }
}

// Stops capturing packets.
func (packetCapture *PacketCapture) StopPacketCapture() {
    packetCapture.handle.Close()
}

// Write captured packets to PCAP file.
// filename: the output PCAP filename that the packets should be written to
// Returns any errors
func (packetCapture *PacketCapture) WriteToPcap(filename string) error {
    // make sure all directories of output path exists
    dir := filepath.Dir(filename)
    err := os.MkdirAll(dir, os.ModePerm)
    if err != nil {
        return err
    }

    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    pcapWriter := pcapgo.NewWriter(file)
    err = pcapWriter.WriteFileHeader(1600, layers.LinkTypeEthernet)
    if err != nil {
        return err
    }
    // write packets to file
    for _, packet := range packetCapture.packets {
        err = pcapWriter.WritePacket(packet.Metadata().CaptureInfo, packet.Data())
        if err != nil {
            return err
        }
    }
    return nil
}
