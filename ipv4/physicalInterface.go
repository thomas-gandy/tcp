package ipv4

import (
	"errors"
	"sync"
)

type DatagramHandler func(datagram *Datagram, pi *PhysicalInterface)

type PhysicalInterface struct {
	Conn                     *Connection
	OnDatagramReceived       DatagramHandler
	addresses                map[Address]struct{}
	addressesMutex           sync.RWMutex
	listenerGoroutinesWg     chan *sync.WaitGroup
	listenerGoroutinesActive bool
}

func NewPhysicalInterface(datagramHandler DatagramHandler) *PhysicalInterface {
	listenerGoroutinesWg := make(chan *sync.WaitGroup, 1)
	listenerGoroutinesWg <- &sync.WaitGroup{}

	physicalInterface := &PhysicalInterface{
		Conn:                 NewSentinelConnection(),
		OnDatagramReceived:   datagramHandler,
		addresses:            make(map[Address]struct{}, 4),
		listenerGoroutinesWg: listenerGoroutinesWg,
	}

	return physicalInterface
}

func (pi *PhysicalInterface) SetConnection(newConnection *Connection) {
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	pi.Conn.Close()
	wg.Wait()
	pi.Conn = newConnection

	if pi.listenerGoroutinesActive {
		pi.listen(wg)
	}
}

func (pi *PhysicalInterface) Send(datagrams []*Datagram) error {
	for _, d := range datagrams {
		select {
		case <-pi.Conn.Done:
			return errors.New("connection is done")
		case pi.Conn.Send <- *d:
		}
	}

	return nil
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
	wg := <-pi.listenerGoroutinesWg
	defer func() {
		pi.listenerGoroutinesWg <- wg
	}()

	pi.Conn.Close()
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
		pi.OnDatagramReceived(datagram, pi)
	} else {
	}
}
