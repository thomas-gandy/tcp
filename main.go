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
	send    chan<- Datagram
	receive <-chan Datagram
	done    chan struct{} // use its close as a broadcast to signal the end of the connection
	destroy sync.Once
}

func (connection *Connection) close() {
	connection.destroy.Do(func() {
		close(connection.done)
	})
}

func createMultiplexedConnection() (*Connection, *Connection) {
	chanA, chanB, done := make(chan Datagram), make(chan Datagram), make(chan struct{})
	aToB := Connection{
		send:    chanA,
		receive: chanB,
		done:    done,
	}
	bToA := Connection{
		send:    chanB,
		receive: chanA,
		done:    done,
	}

	return &aToB, &bToA
}

type PhysicalInterface struct {
	addresses     []uint32 // slice for IP aliasing
	newConnection chan *Connection
	connection    *Connection
}

func (pi *PhysicalInterface) start() {
	for {
		select {
		case datagram := <-pi.connection.receive:
			fmt.Println(datagram)
		case <-pi.connection.done:
			pi.connection.send, pi.connection.receive = nil, nil
		case conn := <-pi.newConnection:
			if pi.connection != nil {
				pi.connection.close()
			}
			pi.connection.send, pi.connection.receive, pi.connection = conn.send, conn.receive, conn
		}
	}
}

type Module struct {
	mutex              sync.RWMutex
	physicalInterfaces map[string]*PhysicalInterface
}

func (module *Module) start() {
	module.mutex.RLock()
	defer module.mutex.RUnlock()

	for _, physicalInterface := range module.physicalInterfaces {
		go physicalInterface.start()
	}
}

func (module *Module) send(d Datagram) error {
	module.mutex.RLock()
	defer module.mutex.RUnlock()

	physicalInterface := module.physicalInterfaces[eth0InterfaceName]
	select {
	case <-physicalInterface.connection.done:
		return errors.New("cannot send datagram as connection has been closed")
	case physicalInterface.connection.send <- d:
	}

	return nil
}

// The IP module for a host
type Host struct {
	Module
	gatewayAddress Address
}

func makeHost() Host {
	return Host{
		Module: Module{sync.RWMutex{}, make(map[string]*PhysicalInterface)},
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
		Module:              Module{sync.RWMutex{}, make(map[string]*PhysicalInterface)},
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

	// Set up send / receive channels between gateway and host
	connA, connB := createMultiplexedConnection()
	gateway.physicalInterfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: connA,
		addresses:  []Address{gateway.address, hostNetworkAddress},
	}
	host.physicalInterfaces[eth0InterfaceName] = &PhysicalInterface{
		connection: connB,
		addresses:  []Address{hostNetworkAddress},
	}

	return nil
}

func (gateway *Gateway) disconnect(interfaceName string) {
	gateway.physicalInterfaces[interfaceName].connection.close()
}

func main() {
	host := makeHost()
	gateway := makeGateway()

	gateway.connect(&host)
	host.start()
	gateway.start()
	gateway.send(Datagram{})

	terminate := make(chan bool)
	go func() {
		time.Sleep(time.Second * 2)
		gateway.disconnect(eth0InterfaceName)
		gateway.send(Datagram{})
		terminate <- true
	}()

	<-terminate
}
