package module

import (
	"errors"
	"fmt"
	"sync"
	"tcp/physicalinterface"
)

// to initially keep things simple, all hosts and gateways will have this single physical interface
const Eth0InterfaceName string = "eth0"

type Module struct {
	physicalInterfacesMutex sync.RWMutex
	physicalInterfaces      map[string]*physicalinterface.PhysicalInterface
}

func newModule() *Module {
	module := Module{sync.RWMutex{}, make(map[string]*physicalinterface.PhysicalInterface)}
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

func (module *Module) PassiveListenOnInterface(interfaceName string) error {
	module.physicalInterfacesMutex.RLock()
	physicalInterface, exists := module.physicalInterfaces[interfaceName]
	module.physicalInterfacesMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no interface with name %s exists", interfaceName)
	}

	physicalInterface.Listen()
	return nil
}

func (module *Module) Send(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("failed to send datagram as send channel has been closed")
		}
	}()

	module.physicalInterfacesMutex.RLock()
	physicalInterface := module.physicalInterfaces[Eth0InterfaceName]
	module.physicalInterfacesMutex.RUnlock()

	header := physicalinterface.Header{}
	datagram := physicalinterface.Datagram{Header: header, Data: data}

	select {
	case <-physicalInterface.Conn.Done:
		return errors.New("cannot send datagram as connection is done")
	case physicalInterface.Conn.Send <- datagram:
	}

	return nil
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
	module.physicalInterfacesMutex.Lock()
	defer module.physicalInterfacesMutex.Unlock()

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
