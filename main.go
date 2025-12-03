package main

import "tcp/ipv4"

func main() {
	defaultInterface := ipv4.Eth0InterfaceName

	host := ipv4.NewHost()
	hostAddress := ipv4.Address(333)
	host.BindAddress(hostAddress, defaultInterface)
	host.PassiveListen(defaultInterface)
	defer host.Stop()

	gateway := ipv4.NewGateway()
	gateway.PassiveListen(defaultInterface)
	defer gateway.Stop()

	ipv4.ConnectModules(gateway.Module, host.Module, defaultInterface, defaultInterface)
	gateway.Send([]byte("aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aa"), hostAddress)
	// gateway.Send([]byte("hello"), hostAddress)
}
