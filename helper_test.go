package dhclient

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseIPs(t *testing.T) {
	assert := assert.New(t)

	data := []byte{143, 209, 4, 1, 143, 209, 5, 1}
	ips := parseIPs(data)
	assert.Len(ips, 2)
	assert.Equal(net.IP{143, 209, 4, 1}, ips[0])
	assert.Equal(net.IP{143, 209, 5, 1}, ips[1])

	// not enough bytes
	assert.Len(parseIPs([]byte{143, 209, 4}), 0)
}
