package main

import (
	"tcp/module"
	"tcp/physicalinterface"
)

func main() {
	defaultInterface := module.Eth0InterfaceName

	host := module.NewHost()
	hostAddress := physicalinterface.Address(333)
	host.BindAddressToInterface(hostAddress, defaultInterface)
	host.PassiveListenOnInterface(defaultInterface)
	defer host.Stop()

	gateway := module.NewGateway()
	gateway.PassiveListenOnInterface(defaultInterface)
	defer gateway.Stop()

	module.ConnectModules(gateway.Module, host.Module, defaultInterface, defaultInterface)
	gateway.Send([]byte("hello"), hostAddress)
}
