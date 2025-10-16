package main

import (
	"tcp/module"
	"tcp/physicalinterface"
)

func main() {
	defaultInterface := module.Eth0InterfaceName

	host := module.NewHost()
	hostAddress := physicalinterface.Address(333)
	host.BindAddress(hostAddress, defaultInterface)
	host.PassiveListen(defaultInterface)
	defer host.Stop()

	gateway := module.NewGateway()
	gateway.PassiveListen(defaultInterface)
	defer gateway.Stop()

	module.ConnectModules(gateway.Module, host.Module, defaultInterface, defaultInterface)
	gateway.Send([]byte("hello"), hostAddress)
}
