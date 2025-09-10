package main

import (
	"errors"
	"fmt"
	"sync"
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
	chanA      chan Datagram
	chanB      chan Datagram
	disconnect sync.Once
}

func (connection *Connection) close() {
	connection.disconnect.Do(func() {
		close(connection.chanA)
		close(connection.chanB)
	})
}

type PhysicalInterface struct {
	connection *Connection
	addresses  []uint32 // slice for IP aliasing
	send       chan<- Datagram
	receive    <-chan Datagram
}

type Module struct {
	physicalInterfaces map[string]*PhysicalInterface
}

func (module *Module) listen() error {
	physicalInterface, ok := module.physicalInterfaces[eth0InterfaceName]
	if !ok {
		return errors.New("module cannot listen as interface uninitialised")
	}
	if physicalInterface.receive == nil {
		return errors.New("module cannot listen as interface receive channel uninitialised")
	}

	for datagram := range physicalInterface.receive {
		fmt.Println(datagram)
	}

	// ensure send channel is also closed
	physicalInterface.connection.close()
	physicalInterface.connection = nil
	physicalInterface.send, physicalInterface.receive = nil, nil

	return nil
}

func (module *Module) send(d Datagram) {
	module.physicalInterfaces[eth0InterfaceName].send <- d
}

// The IP module for a host
type Host struct {
	Module
	gatewayAddress Address
}

func makeHost() Host {
	return Host{
		Module: Module{make(map[string]*PhysicalInterface)},
	}
}

type Address = uint32

// The IP module for a gateway
type Gateway struct {
	Module
	address             Address
	nextFreeHostAddress Address
	connectedHosts      []Address
}

func makeGateway() Gateway {
	return Gateway{
		Module:              Module{make(map[string]*PhysicalInterface)},
		address:             0b0000_1000_0000_0001,
		nextFreeHostAddress: 0b0000_1000_0000_0010,
		connectedHosts:      make([]Address, 0, 64),
	}
}

func (gateway *Gateway) reserveAddress() Address {
	address := gateway.nextFreeHostAddress
	gateway.nextFreeHostAddress++

	return address
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

	host.physicalInterfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: &connection,
		addresses:  []Address{hostNetworkAddress},
		send:       connection.chanA,
		receive:    connection.chanB,
	}

	gateway.physicalInterfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: &connection,
		addresses:  []Address{gateway.address, hostNetworkAddress},
		send:       connection.chanB,
		receive:    connection.chanA,
	}

	return nil
}

func (gateway *Gateway) disconnect(interfaceName string) {
	gateway.physicalInterfaces[interfaceName].connection.close()
	delete(gateway.physicalInterfaces, interfaceName)
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
		time.Sleep(time.Second * 2)
		gateway.disconnect(eth0InterfaceName)
		terminate <- true
	}()

	<-terminate
}
