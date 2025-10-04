package main

import (
	"tcp/module"
	"tcp/physicalinterface"
)

func main() {
	host := module.NewHost()
	defer host.Stop()

	gateway := module.NewGateway()
	defer gateway.Stop()

	defaultInterface := module.Eth0InterfaceName
	module.ConnectModules(gateway.Module, host.Module, defaultInterface, defaultInterface)
	host.PassiveListenOnInterface(defaultInterface)
	gateway.PassiveListenOnInterface(defaultInterface)
	gateway.Send(physicalinterface.Datagram{})
}
