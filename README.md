go-dhclient
===========

`go-dhclient` is a DHCPv4 client library written in Go.
It uses raw sockets and binds them to a specific interface.
Callback functions are triggered on binding or expiration of a lease.

See [main.go](cmd/dhclient/main.go) for example code.

## Dependencies

* [github.com/google/gopacket](https://github.com/google/gopacket) for serializing/unserializing of DHCP packets
* [github.com/mdlayher/raw](https://github.com/mdlayher/raw) for raw sockets
