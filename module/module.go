package module

import (
	"errors"
	"fmt"
	"sync"
	"tcp/physicalinterface"
)

// to initially keep things simple, all hosts and gateways will have this single physical interface
const Eth0InterfaceName string = "eth0"
const minMtuLength uint16 = 576 // currently, ensure is divisble by 8 (has to be for frag offset)

type Module struct {
	physicalInterfacesMutex sync.RWMutex
	physicalInterfaces      map[string]*physicalinterface.PhysicalInterface
	mtuLength               uint16
}

func newModule() *Module {
	module := Module{sync.RWMutex{}, make(map[string]*physicalinterface.PhysicalInterface), 576}
	err := module.addPhysicalInterface(Eth0InterfaceName)
	if err != nil {
		panic("failed to add physical interface when creating a new module")
	}

	return &module
}

func ConnectModules(moduleA, moduleB *Module, interfaceA, interfaceB string) error {
	connA, connB := physicalinterface.CreateMultiplexedConnection()
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

func (module *Module) BindAddress(address physicalinterface.Address, interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	if pi, exists := module.physicalInterfaces[interfaceName]; exists {
		pi.BindAddress(address)
		return nil
	}

	return fmt.Errorf("couldn't bind address to interface %s as it doesn't exist", interfaceName)
}

func (module *Module) UnbindAddress(address physicalinterface.Address, interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	if pi, exists := module.physicalInterfaces[interfaceName]; exists {
		pi.UnbindAddress(address)
		return nil
	}

	return fmt.Errorf("couldn't bind address to interface %s as it doesn't exist", interfaceName)
}

func (module *Module) Send(data []byte, dstAddr physicalinterface.Address) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("send channel closed")
		}
	}()

	module.physicalInterfacesMutex.RLock()
	physicalInterface := module.physicalInterfaces[Eth0InterfaceName]
	module.physicalInterfacesMutex.RUnlock()

	header := physicalinterface.Header{
		DestinationAddress:     dstAddr,
		VersionAndIHL:          0b00000101,
		TotalLength:            0b0101 + uint16(len(data)),
		FlagsAndFragmentOffset: 0b1111_1111_1111_1111,
	}
	header.SetMayFragment(true)

	datagram := physicalinterface.Datagram{Header: header, Data: data}
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

// Will fragment a datagram if the May Fragment flag is set.  If the total length of the datagram
// (as per the total length field) is <= mtu, it will be returned without fragmentation.  Else, the
// max allowed data bytes are extracted; calculated by taking the MTU of the datagram then
// subtracting how much space the header will take (as per the IHL field).  The NFB is calculated
// from the number of 8-byte (byte not bit) blocks in the datagram's underlying data.
func (module *Module) fragment(datagram *physicalinterface.Datagram) ([]*physicalinterface.Datagram, error) {
	if module.mtuLength < minMtuLength {
		return nil, fmt.Errorf("module MTU (%d) below allowed value (%d) ", module.mtuLength, minMtuLength)
	}
	if datagram.Header.TotalLength <= module.mtuLength {
		return []*physicalinterface.Datagram{datagram}, nil
	}

	if !datagram.Header.MayFragment() {
		return nil, errors.New("need to fragment datagram but May Fragment flag not enabled")
	}

	headerLength := uint16(datagram.Header.GetIHL()) * 4
	// ensure *data* in each full fragment is a multiple of 8-bytes
	dataMtuLength := module.mtuLength - headerLength - module.mtuLength%8
	if dataMtuLength <= 0 {
		return nil, fmt.Errorf("MTU is too small and header too large to allow data to be sent")
	}

	totalDataLength := datagram.Header.TotalLength - headerLength
	numOfFullSizedFrags := totalDataLength / dataMtuLength
	fragments := make([]*physicalinterface.Datagram, 0, numOfFullSizedFrags+1)
	byteDataOffset := uint16(0)

	for i := 0; i < int(numOfFullSizedFrags); i++ {
		fragmentHeader := datagram.Header
		fragmentHeader.Identification = 0x00000000000000000000000000000000000000000
		fragmentHeader.SetMoreFragments(true)
		fragmentHeader.SetFragmentOffset(uint16(i * int(dataMtuLength) >> 3))
		fragmentHeader.Options = fragmentHeader.GetFragmentCopyableOptionData()

		fragmentHeader.SetIHL(5 + uint8(len(fragmentHeader.Options)>>2))
		fragmentHeader.TotalLength = module.mtuLength
		fragmentHeader.HeaderChecksum = uint16(fragmentHeader.GetChecksum())

		fragmentData := datagram.Data[byteDataOffset : byteDataOffset+dataMtuLength]
		fragment := &physicalinterface.Datagram{Header: fragmentHeader, Data: fragmentData}
		fragments = append(fragments, fragment)

		byteDataOffset += dataMtuLength
	}

	if byteDataOffset < totalDataLength {
		remainingDataLength := totalDataLength - byteDataOffset

		fragmentHeader := datagram.Header
		fragmentHeader.Identification = 0x00000000000000000000000000000000000000000
		fragmentHeader.SetMoreFragments(false)
		fragmentHeader.SetFragmentOffset(numOfFullSizedFrags * dataMtuLength >> 3)
		fragmentHeader.Options = fragmentHeader.GetFragmentCopyableOptionData()

		fragmentHeader.SetIHL(5 + uint8(len(fragmentHeader.Options)>>2))
		fragmentHeader.TotalLength = fragmentHeader.GetIHLInBytes() + remainingDataLength
		fragmentHeader.HeaderChecksum = fragmentHeader.GetChecksum()

		fragmentData := datagram.Data[byteDataOffset : byteDataOffset+remainingDataLength]
		fragment := &physicalinterface.Datagram{Header: fragmentHeader, Data: fragmentData}
		fragments = append(fragments, fragment)
	} else {
		fragments[len(fragments)-1].Header.SetMoreFragments(false)
	}

	return fragments, nil
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

	physicalInterface := physicalinterface.NewPhysicalInterface()
	module.physicalInterfaces[name] = physicalInterface

	return nil
}

func (module *Module) setInterfaceConnection(interfaceName string, conn *physicalinterface.Connection) error {
	module.physicalInterfacesMutex.RLock()
	defer module.physicalInterfacesMutex.RUnlock()

	physicalInterface, exists := module.physicalInterfaces[interfaceName]
	if !exists {
		return errors.New("interface doesn't exist")
	}
	physicalInterface.SetConnection(conn)

	return nil
}

// The IP module for a host
type Host struct {
	*Module
}

func NewHost() *Host {
	return &Host{Module: newModule()}
}

// The IP module for a gateway
type Gateway struct {
	*Module
}

func NewGateway() *Gateway {
	return &Gateway{
		Module: newModule(),
	}
}
