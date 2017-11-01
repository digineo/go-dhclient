package dhclient

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mdlayher/raw"
)

const responseTimeout = time.Second * 5

const (
	requesting = iota
	renewing
	shutdown
)

// Callback is a function called on certain events
type Callback func(*Lease)

// Client is a DHCP client instance
type Client struct {
	Hostname string
	Iface    *net.Interface
	Lease    *Lease   // The current lease
	OnBound  Callback // On renew or rebound
	OnExpire Callback // On expiration of a lease

	conn   *raw.Conn      // Waw socket
	xid    uint32         // Transaction ID
	wg     sync.WaitGroup // For graceful shutdown
	state  int            // Current operation mode
	mtx    sync.Mutex     // Mutex for the state
	notify chan struct{}
}

// Option is a DHCP option field
type Option struct {
	Type layers.DHCPOpt
	Data []byte
}

// Lease is an assignment by the DHCP server
type Lease struct {
	ServerID     net.IP
	FixedAddress net.IP
	Netmask      net.IPMask
	NextServer   net.IP
	Broadcast    net.IP
	Router       []net.IP
	DNS          []net.IP
	TimeServer   []net.IP
	DomainName   string
	MTU          uint16
	Renew        time.Time
	Rebind       time.Time
	Expire       time.Time
}

// paramsRequestList is a list of params to be requested from the server
var paramsRequestList = []byte{
	1,  // Subnet Mask
	3,  // Router
	4,  // Time Server
	6,  // Domain Name Server
	15, // Domain Name
	26, // Interface MTU
	42, // Network Time Protocol Servers
}

// Start starts the client
func (client *Client) Start() {
	if client.notify != nil {
		log.Panicf("client for %s already started", client.Iface.Name)
	}
	client.notify = make(chan struct{})
	client.wg.Add(1)
	go client.run()
}

// Stop stops the client
func (client *Client) Stop() {
	log.Println("shutting down dhclient for", client.Iface.Name)

	client.setState(shutdown)
	close(client.notify)

	if conn := client.conn; conn != nil {
		conn.Close()
	}
	client.wg.Wait()
}

// Renew triggers the renewal of the current lease
func (client *Client) Renew() {
	client.setState(renewing)
	select {
	case client.notify <- struct{}{}:
	default:
	}
}

// Rebind forgets the current lease and triggers acquirement of a new one
func (client *Client) Rebind() {
	client.setState(requesting)
	client.Lease = nil
	client.Renew()
}

func (client *Client) run() {
	for client.state != shutdown {
		client.runOnce()
	}
	client.wg.Done()
}

func (client *Client) runOnce() {
	var err error
	if client.Lease == nil || client.state == requesting {
		// request new lease
		err = client.withConnection(client.discoverAndRequest)
		if cb := client.OnBound; err == nil && cb != nil {
			cb(client.Lease)
		}
	} else {
		// renew existing lease
		err = client.withConnection(client.request)
	}

	if err != nil {
		log.Println(err)
		// delay for a second
		select {
		case <-client.notify:
		case <-time.After(time.Second):
		}
		return
	}

	select {
	case <-client.notify:
		return
	case <-time.After(time.Until(client.Lease.Expire)):
		// remove lease and request a new one
		client.unbound()
		client.setState(requesting)
	case <-time.After(time.Until(client.Lease.Rebind)):
		// keep lease and request a new one
		client.setState(requesting)
	case <-time.After(time.Until(client.Lease.Renew)):
		// renew the lease
		client.setState(renewing)
	}
}

// setState sets the state if there is no shutdown running
func (client *Client) setState(newState int) {
	client.mtx.Lock()
	if client.state != shutdown {
		client.state = newState
	}
	client.mtx.Unlock()
}

// unbound removes the lease
func (client *Client) unbound() {
	if cb := client.OnExpire; cb != nil {
		cb(client.Lease)
	}
	client.Lease = nil
}

func (client *Client) withConnection(f func() error) error {
	conn, err := raw.ListenPacket(client.Iface, uint16(layers.EthernetTypeIPv4))
	if err != nil {
		return err
	}
	client.conn = conn
	client.xid = rand.Uint32()

	defer func() {
		client.conn.Close()
		client.conn = nil
	}()

	return f()
}

func (client *Client) discoverAndRequest() error {
	err := client.discover()
	if err != nil {
		return err
	}
	return client.request()
}

func (client *Client) discover() error {
	err := client.sendPacket([]Option{
		{layers.DHCPOptMessageType, []byte{byte(layers.DHCPMsgTypeDiscover)}},
		{layers.DHCPOptParamsRequest, paramsRequestList},
		{layers.DHCPOptHostname, []byte(client.Hostname)},
	})

	if err != nil {
		return err
	}

	_, lease, err := client.waitForResponse(layers.DHCPMsgTypeOffer)
	if err != nil {
		return err
	}

	client.Lease = lease
	return nil
}

func (client *Client) request() error {
	err := client.sendPacket([]Option{
		{layers.DHCPOptMessageType, []byte{byte(layers.DHCPMsgTypeRequest)}},
		{layers.DHCPOptParamsRequest, paramsRequestList},
		{layers.DHCPOptHostname, []byte(client.Hostname)},
		{layers.DHCPOptRequestIP, []byte(client.Lease.FixedAddress)},
		{layers.DHCPOptServerID, []byte(client.Lease.ServerID)},
	})

	if err != nil {
		return err
	}

	msgType, lease, err := client.waitForResponse(layers.DHCPMsgTypeAck, layers.DHCPMsgTypeNak)
	if err != nil {
		return err
	}

	switch msgType {
	case layers.DHCPMsgTypeAck:
		if lease.Expire.IsZero() {
			err = errors.New("expire value is zero")
		} else if lease.Renew.IsZero() {
			err = errors.New("renewal value is zero")
		} else if lease.Rebind.IsZero() {
			err = errors.New("rebinding value is zero")
		} else {
			client.Lease = lease
		}
	case layers.DHCPMsgTypeNak:
		err = errors.New("received NAK")
		client.unbound()
	default:
		err = fmt.Errorf("unexpected response: %s", msgType.String())
	}

	return err
}

// sendPacket creates and sends a DHCP packet
func (client *Client) sendPacket(options []Option) error {
	return client.sendMulticast(client.newPacket(options))
}

// newPacket creates a DHCP packet
func (client *Client) newPacket(options []Option) *layers.DHCPv4 {
	packet := layers.DHCPv4{
		Operation:    layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		ClientHWAddr: client.Iface.HardwareAddr,
		Xid:          client.xid, // Transaction ID
	}

	// append DHCP options
	for _, option := range options {
		packet.Options = append(packet.Options, layers.DHCPOption{
			Type:   option.Type,
			Data:   option.Data,
			Length: uint8(len(option.Data)),
		})
	}

	return &packet
}

func (client *Client) sendMulticast(dhcp *layers.DHCPv4) error {
	eth := layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv4,
		SrcMAC:       client.Iface.HardwareAddr,
		DstMAC:       layers.EthernetBroadcast,
	}
	ip := layers.IPv4{
		Version:  4,
		TTL:      64,
		SrcIP:    []byte{0, 0, 0, 0},
		DstIP:    []byte{255, 255, 255, 255},
		Protocol: layers.IPProtocolUDP,
	}
	udp := layers.UDP{
		SrcPort: 68,
		DstPort: 67,
	}

	// Serialize packet
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	udp.SetNetworkLayerForChecksum(&ip)
	err := gopacket.SerializeLayers(buf, opts, &eth, &ip, &udp, dhcp)
	if err != nil {
		return err
	}

	// Send packet
	_, err = client.conn.WriteTo(buf.Bytes(), &raw.Addr{HardwareAddr: eth.DstMAC})
	return err
}

// waitForResponse waits for a DHCP packet with matching transaction ID and the given message type
func (client *Client) waitForResponse(msgTypes ...layers.DHCPMsgType) (layers.DHCPMsgType, *Lease, error) {
	client.conn.SetReadDeadline(time.Now().Add(responseTimeout))
	recvBuf := make([]byte, 1500)
	for {
		_, _, err := client.conn.ReadFrom(recvBuf)
		if err != nil {
			return 0, nil, err
		}

		packet := parsePacket(recvBuf)
		if packet == nil {
			continue
		}

		if packet.Xid == client.xid && packet.Operation == layers.DHCPOpReply {
			msgType, res := newLease(packet)

			// do we have the expected message type?
			for _, t := range msgTypes {
				if t == msgType {
					return msgType, &res, nil
				}
			}
		}
	}
}
