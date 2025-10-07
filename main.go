package main

import (
	"tcp/module"
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
	gateway.Send([]byte("hello"))
}
