// Parses and stores information about the replays
package replay

import (
    "time"
)

// Represents a packet to send to the client
type Packet struct {
    Payload []byte // the bytes to send to the client
    Timestamp time.Duration // time since the start of the replay (first packet received from the cilent) that this packet should be sent
}
