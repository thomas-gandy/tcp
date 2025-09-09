package main

import (
	"errors"
	"fmt"
	"time"
)

// to initially keep things simple, all hosts and gateways will have this single physical interface
const eth0InterfaceName string = "eth0"

type Header struct {
	version            uint8 // with IHL
	typeOfService      uint8
	totalLength        uint16
	identification     uint16
	fragmentOffset     uint16 // with flags
	timeToLive         uint8
	protocol           uint8
	headerChecksum     uint16
	sourceAddress      uint32
	destinationAddress uint32
	options            []uint8
}

type Datagram struct {
	header Header
	data   []uint8
}

// A multiplexed connection representing real-world physical transmission (e.g. wire or wireless)
type Connection struct {
	chanA chan Datagram
	chanB chan Datagram
}

func (connection Connection) close() {
	close(connection.chanA)
	close(connection.chanB)
}

type PhysicalInterface struct {
	connection *Connection
	addresses  []uint32 // slice for IP aliasing
	send       chan<- Datagram
	receive    <-chan Datagram
}

// The IP module for a host
type Host struct {
	gatewayAddress Address
	interfaces     map[string]*PhysicalInterface
}

func makeHost() Host {
	return Host{
		interfaces: make(map[string]*PhysicalInterface),
	}
}

func (host Host) listen() error {
	_, ok := host.interfaces[eth0InterfaceName]
	if !ok {
		return errors.New("host cannot listen as interface uninitialised")
	}
	if host.interfaces[eth0InterfaceName].receive == nil {
		return errors.New("host cannot listen as interface receive channel uninitialised")
	}

	for datagram := range host.interfaces[eth0InterfaceName].receive {
		fmt.Println(datagram)
	}

	return nil
}

type Address = uint32

// The IP module for a gateway
type Gateway struct {
	address             Address
	nextFreeHostAddress Address
	connectedHosts      []Address
	interfaces          map[string]*PhysicalInterface
}

func makeGateway() Gateway {
	return Gateway{
		address:             0b0000_1000_0000_0001,
		nextFreeHostAddress: 0b0000_1000_0000_0010,
		connectedHosts:      make([]Address, 0, 64),
		interfaces:          make(map[string]*PhysicalInterface),
	}
}

func (gateway *Gateway) reserveAddress() Address {
	address := gateway.nextFreeHostAddress
	gateway.nextFreeHostAddress++

	return address
}

func (gateway *Gateway) listen() error {
	_, ok := gateway.interfaces[eth0InterfaceName]
	if !ok {
		return errors.New("gateway cannot listen as interface uninitialised")
	}
	gatewayInterface := gateway.interfaces[eth0InterfaceName]
	if gatewayInterface.receive == nil {
		return errors.New("gateway cannot listen as interface receive channel uninitialised")
	}

	for datagram := range gatewayInterface.receive {
		fmt.Println(datagram)
	}

	// At this point, the above loop would have terminated due to connection channel closure
	// As the connection for the interface has been terminated, remove the interface
	delete(gateway.interfaces, eth0InterfaceName)
	return nil
}

func (gateway *Gateway) send(d Datagram) {
	gateway.interfaces[eth0InterfaceName].send <- d
}

// connect connects a gateway interface to an interface of a host.
//
// It will set the gateway address on the host so it knows where to find the default gateway.
//
// It will bind an address (unique for the network defined by the gateway) to a physical interface
// on the host, which will act as the source IP for sends, and destination IP for receives.  It
// will link up the host's physical interface's send and receive channels between it and the
// gateway's physical interface.
func (gateway *Gateway) connect(host *Host) error {
	hostNetworkAddress := gateway.reserveAddress()
	host.gatewayAddress = gateway.address

	// Set up send / receive channels (a multiplexed connection) between gateway and host
	connection := Connection{
		chanA: make(chan Datagram),
		chanB: make(chan Datagram),
	}

	host.interfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: &connection,
		addresses:  []Address{hostNetworkAddress},
		send:       connection.chanA,
		receive:    connection.chanB,
	}

	gateway.interfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: &connection,
		addresses:  []Address{gateway.address, hostNetworkAddress},
		send:       connection.chanB,
		receive:    connection.chanA,
	}

	return nil
}

func (gateway *Gateway) disconnect() {
	gateway.interfaces[eth0InterfaceName].connection.close()
	delete(gateway.interfaces, eth0InterfaceName)
}

func main() {
	host := makeHost()
	gateway := makeGateway()

	gateway.connect(&host)
	go host.listen()
	go gateway.listen()
	gateway.send(Datagram{})

	terminate := make(chan bool)
	go func() {
		time.Sleep(time.Second * 5)
		gateway.disconnect()
		terminate <- true
	}()

	<-terminate
}
