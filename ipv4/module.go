package ipv4

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// to initially keep things simple, all hosts and gateways will have this single physical interface
const Eth0InterfaceName string = "eth0"
const minMtuLength uint16 = 576 // currently, ensure is divisble by 8 (has to be for frag offset)

type Module struct {
	physicalInterfacesMutex sync.RWMutex
	physicalInterfaces      map[string]*PhysicalInterface
	mtuLength               uint16
	identifierPools         map[IdentifierPoolId]*IdentifierPool
	fragmentBuffers         map[FragmentBufferId]*FragmentBuffer
}

func newModule() *Module {
	module := Module{
		physicalInterfacesMutex: sync.RWMutex{},
		physicalInterfaces:      make(map[string]*PhysicalInterface),
		mtuLength:               576,
		identifierPools:         make(map[IdentifierPoolId]*IdentifierPool),
		fragmentBuffers:         make(map[FragmentBufferId]*FragmentBuffer),
	}

	err := module.addPhysicalInterface(Eth0InterfaceName)
	if err != nil {
		panic("failed to add physical interface when creating a new module")
	}

	return &module
}

func ConnectModules(moduleA, moduleB *Module, interfaceA, interfaceB string) error {
	connA, connB := CreateMultiplexedConnection()
	connected := make(chan struct{}, 2)

	go func() {
		moduleA.setInterfaceConnection(interfaceA, connA)
		connected <- struct{}{}
	}()
	go func() {
		moduleB.setInterfaceConnection(interfaceB, connB)
		connected <- struct{}{}
	}()

	<-connected
	<-connected

	return nil
}

func (module *Module) PassiveListen(interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	physicalInterface, exists := module.physicalInterfaces[interfaceName]
	module.physicalInterfacesMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no interface with name %s exists", interfaceName)
	}

	physicalInterface.Listen()
	return nil
}

func (module *Module) BindAddress(address Address, interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	if pi, exists := module.physicalInterfaces[interfaceName]; exists {
		pi.BindAddress(address)
		return nil
	}

	return fmt.Errorf("couldn't bind address to interface %s as it doesn't exist", interfaceName)
}

func (module *Module) UnbindAddress(address Address, interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	if pi, exists := module.physicalInterfaces[interfaceName]; exists {
		pi.UnbindAddress(address)
		return nil
	}

	return fmt.Errorf("couldn't bind address to interface %s as it doesn't exist", interfaceName)
}

func (module *Module) Send(data []byte, dstAddr Address) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("send channel closed")
		}
	}()

	module.physicalInterfacesMutex.RLock()
	physicalInterface := module.physicalInterfaces[Eth0InterfaceName]
	module.physicalInterfacesMutex.RUnlock()

	idPoolId := IdentifierPoolId{destinationAddress: dstAddr}
	idPool, exists := module.identifierPools[idPoolId]
	if !exists {
		idPool = &IdentifierPool{}
		module.identifierPools[idPoolId] = idPool
	}
	id := idPool.GetNextId()

	header := Header{
		VersionAndIHLFourBytes: 0b00000101,
		Identification:         id,
		TotalLengthBytes:       0b0101 + uint16(len(data)),
		DestinationAddress:     dstAddr,
	}
	header.SetMayFragment(true)

	datagram := Datagram{Header: header, Data: data}
	datagrams, err := module.fragment(&datagram)

	if err != nil {
		return err
	}

	for _, d := range datagrams {
		select {
		case <-physicalInterface.Conn.Done:
			return errors.New("connection is done")
		case physicalInterface.Conn.Send <- *d:
		}
	}

	return nil
}

func (module *Module) Receive(datagram *Datagram) {
	reassembledDatagram, err := module.reassemble(datagram)
	assemblingOccurred := datagram != reassembledDatagram

	if reassembledDatagram != nil && err == nil {
		text := "module has received a full datagram: "
		if assemblingOccurred {
			text = "module has reassembled datagram to: "
		}
		fmt.Println(text, reassembledDatagram)
	}
}

// Will fragment a datagram if the May Fragment flag is set.  If the total length of the datagram
// (as per the total length field) is <= mtu, it will be returned without fragmentation.  Else, the
// max allowed data bytes are extracted; calculated by taking the MTU of the datagram then
// subtracting how much space the header will take (as per the IHL field).  The NFB is calculated
// from the number of 8-byte (byte not bit) blocks in the datagram's underlying data.
func (module *Module) fragment(datagram *Datagram) ([]*Datagram, error) {
	if module.mtuLength < minMtuLength {
		return nil, fmt.Errorf("module MTU (%d) below allowed value (%d) ", module.mtuLength, minMtuLength)
	}
	if datagram.Header.TotalLengthBytes <= module.mtuLength {
		return []*Datagram{datagram}, nil
	}

	if !datagram.Header.MayFragment() {
		return nil, errors.New("need to fragment datagram but May Fragment flag not enabled")
	}

	headerLength := datagram.Header.GetIHLInBytes()
	// ensure *data* in each full fragment is a multiple of 8-bytes
	dataMtuLength := module.mtuLength - headerLength
	boundedDataMtuLength := dataMtuLength - (dataMtuLength % 8)
	if boundedDataMtuLength <= 0 {
		return nil, fmt.Errorf("MTU is too small and header too large to allow data to be sent")
	}

	totalDataLength := datagram.Header.TotalLengthBytes - headerLength
	numOfFullSizedFrags := totalDataLength / boundedDataMtuLength
	fragments := make([]*Datagram, 0, numOfFullSizedFrags+1)
	byteDataOffset := uint16(0)

	for i := range numOfFullSizedFrags {
		fragmentHeader := datagram.Header
		fragmentHeader.SetMoreFragments(true)
		fragmentHeader.SetFragmentOffset(i * boundedDataMtuLength >> 3)
		fragmentHeader.Options = fragmentHeader.GetFragmentCopyableOptionData()

		fragmentHeader.SetIHL(5 + uint8(len(fragmentHeader.Options)>>2))
		fragmentHeader.TotalLengthBytes = boundedDataMtuLength + fragmentHeader.GetIHLInBytes()
		fragmentHeader.HeaderChecksum = fragmentHeader.GetChecksum()

		fragmentData := datagram.Data[byteDataOffset : byteDataOffset+boundedDataMtuLength]
		fragment := &Datagram{Header: fragmentHeader, Data: fragmentData}
		fragments = append(fragments, fragment)

		byteDataOffset += boundedDataMtuLength
	}

	if byteDataOffset < totalDataLength {
		remainingDataLength := totalDataLength - byteDataOffset

		fragmentHeader := datagram.Header
		fragmentHeader.SetMoreFragments(false)
		fragmentHeader.SetFragmentOffset(numOfFullSizedFrags * boundedDataMtuLength >> 3)
		fragmentHeader.Options = fragmentHeader.GetFragmentCopyableOptionData()

		fragmentHeader.SetIHL(5 + uint8(len(fragmentHeader.Options)>>2))
		fragmentHeader.TotalLengthBytes = fragmentHeader.GetIHLInBytes() + remainingDataLength
		fragmentHeader.HeaderChecksum = fragmentHeader.GetChecksum()

		fragmentData := datagram.Data[byteDataOffset : byteDataOffset+remainingDataLength]
		fragment := &Datagram{Header: fragmentHeader, Data: fragmentData}
		fragments = append(fragments, fragment)
	} else {
		fragments[len(fragments)-1].Header.SetMoreFragments(false)
	}

	return fragments, nil
}

// If this fragment does not complete the datagram but is successfully inserted into the fragment
// buffer, (nil, nil) will be returned.  If the fragment does complete the datagram, the complete
// datagram will be returned.
func (module *Module) reassemble(fragment *Datagram) (*Datagram, error) {
	header := fragment.Header
	fragmentBufferId := makeFragmentBufferId(&fragment.Header)

	if !header.MoreFragments() && header.GetFragmentOffset() == 0 {
		delete(module.fragmentBuffers, fragmentBufferId)
		return fragment, nil
	}

	fragmentBuffer, exists := module.fragmentBuffers[fragmentBufferId]
	if !exists {
		fragmentBuffer = newFragmentBuffer()
		fragmentBuffer.startUnixSecs = time.Now().Unix()
		module.fragmentBuffers[fragmentBufferId] = fragmentBuffer
	}
	reassembledDatagram := fragmentBuffer.Insert(fragment)
	if reassembledDatagram != nil {
		delete(module.fragmentBuffers, fragmentBufferId)
	}

	return reassembledDatagram, nil
}

func (module *Module) Stop() {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	wg := sync.WaitGroup{}
	for _, physicalInterface := range module.physicalInterfaces {
		wg.Go(physicalInterface.Stop)
	}
	wg.Wait()
}

func (module *Module) addPhysicalInterface(name string) error {
	module.physicalInterfacesMutex.Lock()
	defer module.physicalInterfacesMutex.Unlock()

	_, exists := module.physicalInterfaces[name]
	if exists {
		return fmt.Errorf("interface %s already exists", name)
	}

	physicalInterface := NewPhysicalInterface(module.Receive)
	module.physicalInterfaces[name] = physicalInterface

	return nil
}

func (module *Module) setInterfaceConnection(interfaceName string, conn *Connection) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	physicalInterface, exists := module.physicalInterfaces[interfaceName]
	if !exists {
		return errors.New("interface doesn't exist")
	}
	physicalInterface.SetConnection(conn)

	return nil
}

type Host struct {
	*Module
}

func NewHost() *Host {
	return &Host{Module: newModule()}
}

type Gateway struct {
	*Module
}

func NewGateway() *Gateway {
	return &Gateway{
		Module: newModule(),
	}
}
