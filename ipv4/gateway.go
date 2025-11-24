package ipv4

type Gateway struct {
	*Module
}

func NewGateway() *Gateway {
	return &Gateway{
		Module: newModule(),
	}
}
