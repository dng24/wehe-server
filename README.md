# New Wehe server

Things that are done
- server initialization
- side channel
- tcp replays
- udp replays
- analyzer
- old side channel backwards compatibility
- tls

List of TODOs
- allow requests for different analyses
- should side channel use binary buffers instead of string?
  - combine receive id and ask4permission
- confirmation replays
- finish implementing pcap capture of replay traffic
- use first packet to detect replay instead of IP
- implement "replay store" where users/servers can download/update/delete replays
  - update certs as well
- timeouts
- double check tcp and udp receiving all bytes its supposed to on conn.Read
- add logging
  - print out stack trace on errors
- figure out how to change root ca cert without losing too many clients
- write unit tests
- fix all the todos scattered in the code
- more testing
- write documentation
- even more testing
