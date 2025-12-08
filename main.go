package main

import (
	"tcp/ipv4"
)

func main() {
	defaultInterface := ipv4.Eth0InterfaceName
	hostAddress := ipv4.Address(333)
	gatewayAddress := ipv4.Address(334)

	host := ipv4.NewHost()
	host.BindAddress(hostAddress, defaultInterface)
	host.PassiveListen(defaultInterface)
	host.AddToRouteTable(gatewayAddress, defaultInterface)
	defer host.Stop()

	gateway := ipv4.NewGateway()
	gateway.BindAddress(gatewayAddress, defaultInterface)
	gateway.PassiveListen(defaultInterface)
	gateway.AddToRouteTable(hostAddress, defaultInterface)
	defer gateway.Stop()

	ipv4.ConnectModules(gateway.Module, host.Module, defaultInterface, defaultInterface)

	gateway.Send([]byte("abcd"), hostAddress)
	gateway.Send([]byte("aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aaaaa11111aa"), hostAddress)
	for range 1 << 19 {
		gateway.Send([]byte("hello"), hostAddress)
	}
}
