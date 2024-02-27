// Parses and stores information about the replays
package replay

// Represents a packet to send to the client
type Packet struct {
    Payload []byte // the bytes to send to the client
    Timestamp float64 // time since the start of the replay that this packet should be sent
}
