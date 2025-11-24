package ipv4

type Host struct {
	*Module
}

func NewHost() *Host {
	return &Host{Module: newModule()}
}
