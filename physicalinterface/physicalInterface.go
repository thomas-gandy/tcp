package physicalinterface

import (
	"fmt"
	"sync"
)

type Header struct {
	Version            uint8 // with IHL
	TypeOfService      uint8
	TotalLength        uint16
	Identification     uint16
	FragmentOffset     uint16 // with flags
	TimeToLive         uint8
	Protocol           uint8
	HeaderChecksum     uint16
	SourceAddress      uint32
	DestinationAddress uint32
	Options            []uint8
}

type Datagram struct {
	Header Header
	Data   []uint8
}

type Address = uint32

// A multiplexed connection representing real-world physical transmission (e.g. wire or wireless)
type Connection struct {
	Send    chan<- Datagram
	Receive <-chan Datagram
	Done    chan struct{} // use its close as a broadcast to signal the end of the connection
	destroy *sync.Once
}

func newSentinelConnection() *Connection {
	closedSendChannel := make(chan Datagram)
	close(closedSendChannel)

	sentinelConnection := &Connection{
		Send:    closedSendChannel,
		Receive: nil,
		Done:    make(chan struct{}),
		destroy: &sync.Once{},
	}

	return sentinelConnection
}

func (connection *Connection) Close() {
	connection.destroy.Do(func() {
		if connection.Done != nil {
			close(connection.Done)
		}
	})
}

func CreateMultiplexedConnection() (*Connection, *Connection) {
	chanA, chanB := make(chan Datagram), make(chan Datagram)
	done, destroy := make(chan struct{}), &sync.Once{}
	aToB := Connection{
		Send:    chanA,
		Receive: chanB,
		Done:    done,
		destroy: destroy,
	}
	bToA := Connection{
		Send:    chanB,
		Receive: chanA,
		Done:    done,
		destroy: destroy,
	}

	return &aToB, &bToA
}

type PhysicalInterface struct {
	Conn                     *Connection
	addresses                map[Address]struct{}
	addressesMutex           sync.RWMutex
	listenerGoroutinesWg     chan *sync.WaitGroup
	listenerGoroutinesActive bool
}

func NewPhysicalInterface() *PhysicalInterface {
	listenerGoroutinesWg := make(chan *sync.WaitGroup, 1)
	listenerGoroutinesWg <- &sync.WaitGroup{}

	physicalInterface := &PhysicalInterface{
		Conn:                 newSentinelConnection(),
		addresses:            make(map[Address]struct{}, 4),
		listenerGoroutinesWg: listenerGoroutinesWg,
	}

	return physicalInterface
}

func (pi *PhysicalInterface) SetConnection(newConnection *Connection) {
	pi.Conn.Close()
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	wg.Wait()
	pi.Conn = newConnection

	if pi.listenerGoroutinesActive {
		pi.listen(wg)
	}
}

func (pi *PhysicalInterface) Listen() {
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	if !pi.listenerGoroutinesActive {
		pi.listen(wg)
	}
}

func (pi *PhysicalInterface) BindAddress(address Address) {
	pi.addressesMutex.Lock()
	defer pi.addressesMutex.Unlock()

	pi.addresses[address] = struct{}{}
}

func (pi *PhysicalInterface) UnbindAddress(address Address) {
	pi.addressesMutex.Lock()
	defer pi.addressesMutex.Unlock()

	delete(pi.addresses, address)
}

func (pi *PhysicalInterface) Stop() {
	pi.Conn.Close()

	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	wg.Wait()
	pi.listenerGoroutinesActive = false
}

func (pi *PhysicalInterface) passiveListenDatagram() {
	for {
		select {
		case datagram := <-pi.Conn.Receive:
			pi.handleDatagram(&datagram)
		case <-pi.Conn.Done:
			return
		}
	}
}

func (pi *PhysicalInterface) passiveListenConnectionDone() {
	<-pi.Conn.Done
	pi.Conn.Send, pi.Conn.Receive = nil, nil
}

func (pi *PhysicalInterface) listen(wg *sync.WaitGroup) {
	wg.Go(pi.passiveListenDatagram)
	wg.Go(pi.passiveListenConnectionDone)
	pi.listenerGoroutinesActive = true
}

func (pi *PhysicalInterface) handleDatagram(datagram *Datagram) {
	pi.addressesMutex.RLock()
	defer pi.addressesMutex.RUnlock()

	if _, exists := pi.addresses[datagram.Header.DestinationAddress]; exists {
		fmt.Println("received datagram: ", datagram)
	} else {
		fmt.Println("rejected datagram: ", datagram)
	}
}
