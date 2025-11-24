package ipv4

import (
	"sync"
)

// A multiplexed connection representing real-world physical transmission (e.g. wire or wireless)
type Connection struct {
	Send    chan<- Datagram
	Receive <-chan Datagram
	Done    chan struct{} // use its close as a broadcast to signal the end of the connection
	destroy *sync.Once
}

func NewSentinelConnection() *Connection {
	closedSendChannel := make(chan Datagram)
	close(closedSendChannel)

	sentinelConnection := &Connection{
		Send:    closedSendChannel,
		Receive: nil,
		Done:    make(chan struct{}),
		destroy: &sync.Once{},
	}

	return sentinelConnection
}

func (connection *Connection) Close() {
	connection.destroy.Do(func() {
		if connection.Done != nil {
			close(connection.Done)
		}
	})
}

func CreateMultiplexedConnection() (*Connection, *Connection) {
	chanA, chanB := make(chan Datagram), make(chan Datagram)
	done, destroy := make(chan struct{}), &sync.Once{}
	aToB := Connection{
		Send:    chanA,
		Receive: chanB,
		Done:    done,
		destroy: destroy,
	}
	bToA := Connection{
		Send:    chanB,
		Receive: chanA,
		Done:    done,
		destroy: destroy,
	}

	return &aToB, &bToA
}
