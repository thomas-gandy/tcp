package main

const (
	StateListen = iota
	StateSynSent
	StateSynReceived
	StateEstablished
	StateFinWait1
	StateFinWait2
	StateCloseWait
	StateClosing
	StateLastAck
	StateTimeWait
	StateClosed
	TotalStates
)

const (
	ActionActiveOpen = iota
	ActionPassiveOpen
	ActionClose
	ActionReceiveSyn
	ActionReceiveRst
	ActionSend
	ActionReceiveAckOfSyn
	ActionReceiveSynAck
	ActionReceiveFin
	ActionReceiveAckOfFin
	ActionTimeout
	TotalActions
)

var stateTransitionTable = generateStateActionResultTransitionMap()

type StateMachine struct {
	CurrentState int
}

func generateStateActionResultTransitionMap() map[int]map[int]int {
	result := make(map[int]map[int]int, TotalStates)

	result[StateClosed] = make(map[int]int, TotalActions)
	result[StateClosed][ActionActiveOpen] = StateSynSent
	result[StateClosed][ActionPassiveOpen] = StateListen

	result[StateListen] = make(map[int]int, TotalActions)
	result[StateListen][ActionClose] = StateClosed
	result[StateListen][ActionReceiveSyn] = StateSynReceived
	result[StateListen][ActionSend] = StateSynSent

	result[StateSynReceived] = make(map[int]int, TotalActions)
	result[StateSynReceived][ActionReceiveRst] = StateListen
	result[StateSynReceived][ActionReceiveAckOfSyn] = StateEstablished
	result[StateSynReceived][ActionClose] = StateFinWait1

	result[StateSynSent] = make(map[int]int, TotalActions)
	result[StateSynSent][ActionClose] = StateClosed
	result[StateSynSent][ActionReceiveSyn] = StateSynReceived
	result[StateSynSent][ActionReceiveSynAck] = StateEstablished

	result[StateEstablished] = make(map[int]int, TotalActions)
	result[StateEstablished][ActionClose] = StateFinWait1
	result[StateEstablished][ActionReceiveFin] = StateCloseWait

	result[StateFinWait1] = make(map[int]int, TotalActions)
	result[StateFinWait1][ActionReceiveFin] = StateClosing
	result[StateFinWait1][ActionReceiveAckOfFin] = StateFinWait2

	result[StateCloseWait] = make(map[int]int, TotalActions)
	result[StateCloseWait][ActionClose] = StateLastAck

	result[StateFinWait2] = make(map[int]int, TotalActions)
	result[StateFinWait2][ActionReceiveFin] = StateTimeWait

	result[StateClosing] = make(map[int]int, TotalActions)
	result[StateClosing][ActionReceiveAckOfFin] = StateTimeWait

	result[StateLastAck] = make(map[int]int, TotalActions)
	result[StateLastAck][ActionReceiveAckOfFin] = StateClosed

	result[StateTimeWait] = make(map[int]int, TotalActions)
	result[StateTimeWait][ActionTimeout] = StateClosed

	return result
}
