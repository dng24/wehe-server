# New Wehe server

Things that are done
- server initialization
- side channel
- tcp replays
- udp replays

List of TODOs
- finish analyzer
  - allow requests for different analyses
- fix mlab prefix
- should side channel use binary buffers instead of string?
- confirmation replays
- finish implementing pcap capture of replay traffic
- use first packet to detect replay instead of IP
- implement encryption/certs for side channel
- implement "replay store" where users/servers can download/update/delete replays
- implement backwards compatible server so that old clients can still run
- add logging
  - print out stack trace on errors
- write unit tests
- fix all the todos scattered in the code
- more testing
- write documentation
- even more testing
