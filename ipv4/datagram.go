package ipv4

type Datagram struct {
	Header Header
	Data   []byte
}
