package physicalinterface

import (
	"fmt"
	"sync"
	"unsafe"
)

const (
	fragmentOffsetBitLength               = 13
	fragmentOffsetBitMask                 = 0b0001_1111_1111_1111
	FlagDontFragment               uint16 = 0b010 << fragmentOffsetBitLength
	flagMoreFragments              uint16 = 0b001 << fragmentOffsetBitLength
	headerByteLengthWithoutOptions        = 20

	HeaderOptionTypeCopiedFlagMask = 0b1000_0000
	HeaderOptionTypeClassMask      = 0b0110_0000
	HeaderOptionTypeNumberMask     = 0b0001_1111

	HeaderOptionEndOfOptions        = 0b0000_0000
	HeaderOptionNoOperation         = 0b0000_0000
	HeaderOptionSecurity            = 0b0000_0000
	HeaderOptionLooseSourceRouting  = 0b0000_0000
	HeaderOptionStrictSourceRouting = 0b0000_0000
	HeaderOptionRecordRoute         = 0b0000_0000
	HeaderOptionStreamId            = 0b0000_0000
	HeaderOptionInternetTimestamp   = 0b0000_0000
)

type Header struct {
	VersionAndIHL          uint8 // version (4-bits) and IHL (4-bits)
	TypeOfService          uint8
	TotalLength            uint16
	Identification         uint16
	FlagsAndFragmentOffset uint16 // flags (3-bits) and fragment offset (13-bits)
	TimeToLive             uint8
	Protocol               uint8
	HeaderChecksum         uint16
	SourceAddress          uint32
	DestinationAddress     uint32
	Options                []uint8
}

func (h *Header) SetVersion(version uint8) {
	h.VersionAndIHL |= version << 4
}

func (h *Header) SetIHL(dwords uint8) {
	h.VersionAndIHL |= dwords
}

func (h *Header) GetIHL() uint8 {
	return h.VersionAndIHL & 0b0000_1111
}

func (h *Header) GetIHLInBytes() uint16 {
	return uint16(h.VersionAndIHL&0b0000_1111) * 4
}

func (h *Header) MayFragment() bool {
	return h.FlagsAndFragmentOffset&FlagDontFragment == 0
}

func (h *Header) SetMayFragment(allowFragmentation bool) {
	h.setFlagState(FlagDontFragment, !allowFragmentation)
}

func (h *Header) MoreFragments() bool {
	return h.FlagsAndFragmentOffset&flagMoreFragments == 1
}

func (h *Header) SetMoreFragments(moreFragments bool) {
	h.setFlagState(flagMoreFragments, moreFragments)
}

func (h *Header) SetFragmentOffset(offset uint16) {
	h.FlagsAndFragmentOffset &^= fragmentOffsetBitMask
	h.FlagsAndFragmentOffset |= offset & fragmentOffsetBitMask
}

func (h *Header) GetFragmentOffset() uint16 {
	return h.FlagsAndFragmentOffset & fragmentOffsetBitMask
}

func (h *Header) GetChecksum() uint16 {
	data := (*uint16)(unsafe.Pointer(h))
	chunkCount := int(h.GetIHL()) * 2 // 16-bit chunk sizes
	var sum uint16 = 0

	for range 5 {
		sum = onesComplementSum(sum, *data)
		data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16))
	}

	data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16)) // skip checksum itself

	for i := 6; i < chunkCount; i++ {
		sum = onesComplementSum(sum, *data)
		data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16))
	}

	return ^sum
}

func (h *Header) GetFragmentCopyableOptionData() []uint8 {
	ihlByteCount := h.GetIHLInBytes()
	optionsCount := uint8(ihlByteCount - headerByteLengthWithoutOptions)
	if optionsCount == 0 {
		return make([]uint8, 0)
	}

	optionTypeIndex := uint8(0)
	result := make([]uint8, 0, optionsCount)
	for optionTypeIndex < optionsCount {
		optionType := h.Options[optionTypeIndex]

		if optionType == 0 {
			break
		}

		if optionType&HeaderOptionTypeCopiedFlagMask == 1 {
			number := optionType & HeaderOptionTypeNumberMask

			if number >= 2 {
				optionLength := h.Options[optionTypeIndex+1]
				result = append(result, h.Options[optionTypeIndex:optionLength]...)
				optionTypeIndex += optionLength
			} else {
				result = append(result, optionType)
				optionTypeIndex++
			}
		}
	}

	return result
}

func onesComplementSum(a, b uint16) uint16 {
	var result uint32 = uint32(a) + uint32(b)
	var carry uint16 = uint16(result>>16) & 1

	return uint16(result&0xFFFF) + carry
}

func (h *Header) setFlagState(flag uint16, enable bool) {
	if enable {
		h.FlagsAndFragmentOffset |= flag
	} else {
		h.FlagsAndFragmentOffset &^= flag
	}
}

type Datagram struct {
	Header Header
	Data   []byte
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
		fmt.Println("received datagram: ", datagram)
	} else {
		fmt.Println("rejected datagram: ", datagram)
	}
}
