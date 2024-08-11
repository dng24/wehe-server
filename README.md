# New Wehe server

Things that are done
- server initialization
- side channel
- tcp replays
- udp replays
- analyzer
- old side channel backwards compatibility

List of TODOs
- fix first message
- allow requests for different analyses
- make sure all conn.Read are actually getting the full message
- should side channel use binary buffers instead of string?
- confirmation replays
- finish implementing pcap capture of replay traffic
- use first packet to detect replay instead of IP
- implement encryption/certs for side channel
- implement "replay store" where users/servers can download/update/delete replays
 - update certs as well
- add logging
  - print out stack trace on errors
- write unit tests
- fix all the todos scattered in the code
- more testing
- write documentation
- even more testing
