package main

import (
	"errors"
	"fmt"
	"sync"
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
	destroy *sync.Once
}

func (connection *Connection) close() {
	connection.destroy.Do(func() {
		if connection.done != nil {
			close(connection.done)
		}
	})
}

func createMultiplexedConnection() (*Connection, *Connection) {
	chanA, chanB := make(chan Datagram), make(chan Datagram)
	done, destroy := make(chan struct{}), &sync.Once{}
	aToB := Connection{
		send:    chanA,
		receive: chanB,
		done:    done,
		destroy: destroy,
	}
	bToA := Connection{
		send:    chanB,
		receive: chanA,
		done:    done,
		destroy: destroy,
	}

	return &aToB, &bToA
}

type PhysicalInterface struct {
	conn                 *Connection
	listenerGoroutinesWg chan *sync.WaitGroup
}

func newPhysicalInterface() *PhysicalInterface {
	physicalInterface := &PhysicalInterface{
		conn: &Connection{
			send:    make(chan Datagram), // this will be immediately closed
			receive: nil,
			done:    make(chan struct{}),
			destroy: &sync.Once{},
		},
		listenerGoroutinesWg: make(chan *sync.WaitGroup, 1),
	}
	close(physicalInterface.conn.send)
	physicalInterface.listenerGoroutinesWg <- &sync.WaitGroup{}

	return physicalInterface
}

func (pi *PhysicalInterface) Listen() {
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	pi.listen(wg)
}

func (pi *PhysicalInterface) listen(wg *sync.WaitGroup) {
	wg.Wait() // ensure only one listen routine occurs at any given time
	wg.Go(pi.passiveListenDatagram)
	wg.Go(pi.passiveListenConnectionDone)
}

func (pi *PhysicalInterface) passiveListenDatagram() {
	for {
		select {
		case datagram := <-pi.conn.receive:
			fmt.Println(datagram)
		case <-pi.conn.done:
			return
		}
	}
}

func (pi *PhysicalInterface) passiveListenConnectionDone() {
	<-pi.conn.done
	pi.conn.send, pi.conn.receive = nil, nil
}

func (pi *PhysicalInterface) stop() {
	pi.conn.close()

	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	wg.Wait()
}

func (pi *PhysicalInterface) setConnection(newConnection *Connection) {
	pi.conn.close()
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	wg.Wait()
	pi.conn = newConnection
	pi.listen(wg)
}

type Module struct {
	physicalInterfacesMutex sync.RWMutex
	physicalInterfaces      map[string]*PhysicalInterface
}

func newModule() *Module {
	module := Module{sync.RWMutex{}, make(map[string]*PhysicalInterface)}
	err := module.addPhysicalInterface(eth0InterfaceName)
	if err != nil {
		panic("failed to add physical interface when creating a new module")
	}

	return &module
}

func (module *Module) addPhysicalInterface(name string) error {
	module.physicalInterfacesMutex.Lock()
	defer module.physicalInterfacesMutex.Unlock()

	_, exists := module.physicalInterfaces[name]
	if exists {
		return fmt.Errorf("interface %s already exists", name)
	}

	physicalInterface := newPhysicalInterface()
	module.physicalInterfaces[name] = physicalInterface

	return nil
}

func (module *Module) setInterfaceConnection(interfaceName string, conn *Connection) error {
	module.physicalInterfacesMutex.Lock()
	defer module.physicalInterfacesMutex.Unlock()

	physicalInterface, exists := module.physicalInterfaces[interfaceName]
	if !exists {
		return errors.New("interface doesn't exist")
	}
	physicalInterface.setConnection(conn)

	return nil
}

func (module *Module) stop() {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	wg := sync.WaitGroup{}
	for _, physicalInterface := range module.physicalInterfaces {
		wg.Go(physicalInterface.stop)
	}
	wg.Wait()
}

func (module *Module) send(d Datagram) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("failed to send datagram as send channel has been closed")
		}
	}()

	module.physicalInterfacesMutex.RLock()
	physicalInterface := module.physicalInterfaces[eth0InterfaceName]
	module.physicalInterfacesMutex.RUnlock()

	select {
	case <-physicalInterface.conn.done:
		return errors.New("cannot send datagram as connection has been closed")
	case physicalInterface.conn.send <- d:
	}

	return nil
}

// The IP module for a host
type Host struct {
	*Module
}

func newHost() *Host {
	return &Host{Module: newModule()}
}

type Address = uint32

// The IP module for a gateway
type Gateway struct {
	*Module
}

func newGateway() *Gateway {
	return &Gateway{
		Module: newModule(),
	}
}

func (gateway *Gateway) connectHost(host *Host, gInterface, hInterface string) error {
	connA, connB := createMultiplexedConnection()
	connected := make(chan struct{}, 2)

	go func() {
		gateway.setInterfaceConnection(gInterface, connA)
		connected <- struct{}{}
	}()
	go func() {
		host.setInterfaceConnection(hInterface, connB)
		connected <- struct{}{}
	}()

	<-connected
	<-connected

	return nil
}

func main() {
	host := newHost()
	defer host.stop()

	gateway := newGateway()
	defer gateway.stop()

	gateway.connectHost(host, eth0InterfaceName, eth0InterfaceName)
	gateway.send(Datagram{})
}
