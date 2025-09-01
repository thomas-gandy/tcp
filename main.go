package main

import "fmt"

type Header struct {
	version            uint8 // with IHL
	typeOfService      uint8
	totalLength        uint16
	identification     uint16
	fragmentOffset     uint16 // with flags
	timeToLive         uint8
	protocol           uint8
	headerChecksum     uint16
	sourceAddress      uint32
	destinationAddress uint32
	options            []uint8
}

type Datagram struct {
	header Header
	data   []uint8
}

type Wire struct {
	send    chan Datagram
	receive chan Datagram
}

type Host struct {
	MacAddress       string
	GatewayInterface Wire
}

func (host Host) listen() {
	for datagram := range host.GatewayInterface.receive {
		fmt.Println(datagram)
	}
}

type Gateway struct {
	interfaces map[string]Wire
}

func (gateway Gateway) listen() {
	for _, gatewayInterface := range gateway.interfaces {
		for datagram := range gatewayInterface.receive {
			fmt.Println(datagram)
		}
	}
}

func (gateway *Gateway) connect(hosts ...*Host) {
	for _, host := range hosts {
		host.GatewayInterface = Wire{
			send:    make(chan Datagram),
			receive: make(chan Datagram),
		}
		gateway.interfaces[host.MacAddress] = Wire{
			send:    host.GatewayInterface.receive,
			receive: host.GatewayInterface.send,
		}
	}
}

func main() {
	hosts := []*Host{{MacAddress: "hostA"}, {MacAddress: "hostB"}}
	gateway := Gateway{interfaces: make(map[string]Wire)}
	gateway.connect(hosts...)

	for _, host := range hosts {
		go host.listen()
	}
	go gateway.listen()

}
