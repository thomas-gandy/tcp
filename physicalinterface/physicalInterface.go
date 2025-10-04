package physicalinterface

import (
	"fmt"
	"sync"
)

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
	Send    chan<- Datagram
	Receive <-chan Datagram
	Done    chan struct{} // use its close as a broadcast to signal the end of the connection
	destroy *sync.Once
}

func NewSentinelConnection() *Connection {
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
	listenerGoroutinesWg     chan *sync.WaitGroup
	listenerGoroutinesActive bool
}

func NewPhysicalInterface() *PhysicalInterface {
	listenerGoroutinesWg := make(chan *sync.WaitGroup, 1)
	listenerGoroutinesWg <- &sync.WaitGroup{}

	physicalInterface := &PhysicalInterface{
		Conn:                 NewSentinelConnection(),
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
	pi.listenerGoroutinesActive = false
}

func (pi *PhysicalInterface) Listen() {
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	if pi.listenerGoroutinesActive {
		return
	}

	wg.Wait() // ensure only one listen routine occurs at any given time
	wg.Go(pi.passiveListenDatagram)
	wg.Go(pi.passiveListenConnectionDone)
	pi.listenerGoroutinesActive = true
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
			fmt.Println(datagram)
		case <-pi.Conn.Done:
			return
		}
	}
}

func (pi *PhysicalInterface) passiveListenConnectionDone() {
	<-pi.Conn.Done
	pi.Conn.Send, pi.Conn.Receive = nil, nil
}
