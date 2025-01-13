package main

import (
	"errors"
	"fmt"
	"math/rand"
)

const (
	OptionEndOfList      = 0 // used at end of all options (additional padding could occur after last option)
	OptionNoOperation    = 1 // can be used to align a subsequent option on a certain word boundary (not necessarily)
	OptionMaxSegmentSize = 2 // maximum segment size the sender can receive
)

const (
	ControlBitMaskCongestionWindowReduced = 1 << 7
	ControlBitMaskECNEcho                 = 1 << 6
	ControlBitMaskUrgentPointer           = 1 << 5
	ControlBitMaskAck                     = 1 << 4
	ControlBitMaskPushFunction            = 1 << 3
	ControlBitMaskReset                   = 1 << 2
	ControlBitMaskSyn                     = 1 << 1
	ControlBitMaskFin                     = 1
)

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

type TransmissionControlBlock struct {
	LocalIP, DestinationIP     uint32
	LocalPort, DestinationPort uint16
	ReceiveBuffer, SendBuffer  []byte
	RetransmissionQueue        []*TcpSegment
	CurrentSegment             *TcpSegment

	SendUnAck               uint32
	SendNext                uint32
	SendWindow              bool
	SendUrgentPointer       bool
	SendSeqNumLastWinUpdate uint32
	SendAckNumLastWinUpdate uint32
	SendInitialSendSeqNum   uint32

	ReceiveNext                 uint32
	ReceiveWindow               uint32
	ReceiveUrgentPointer        bool
	ReceiveInitialReceiveSeqNum uint32
}

type TcpHeader struct {
	sourcePort, destinationPort uint16

	/*
		Sequence number of first octet (byte) in segment, or initial sequence number when SYN bit set
		Expected next sequence number for sender to receive when ACK bit set
	*/
	seqNumber, ackNumber uint32

	/*
		Number of 32 bit (4 byte) words in header.
		The first four bits are the actual data offset, the rest are reserved.
		Number of bytes in header up to (excluding) options is 20, so offset to final word would be 0101 (5).
		If dataOffset > 5, then options are present and is (dataOffset - 5) * 4 bytes long.
	*/
	dataOffset  uint8
	controlBits uint8

	/*
		Number of bytes (starting with first ACK byte) the sender is willing to receive.
	*/
	window uint16

	/*
		16-bit ones' complement of ones' complement sum of all 16-bit words in header and text.
		If segment has odd number of bytes, can pad last byte with zeroed byte.
		Pseudo header (for IPv4 xor IPv6) used to make checksum but not actually sent with segment.
	*/
	checksum      uint16
	urgentPointer uint16

	/*
		Multiple of 8-bits (1 byte), with its byte size calculated via (dataOffset - 5) * 4 (may have 3 byte padding).
		An option can be a single byte which represents a kind of option (like a flag).
		Or an option can be a byte for its option-kind followed by a byte representing option-length, followed by data.
		This is like it stating a function, its arguments, and the length of its arguments.
		The option-length includes the option-kind and option-length bytes.
	*/
	options []int8
}

type TcpSegment struct {
	Header TcpHeader
	Data   []byte
}

type StateMachine struct {
	CurrentState int
}

func (machine *StateMachine) Transition(action int) error {
	availableActionMap, ok := stateTransitionTable[machine.CurrentState]
	if !ok {
		return errors.New("unknown state")
	}

	newState, ok := availableActionMap[action]
	if !ok {
		return errors.New("no action defined for the current state")
	}

	machine.CurrentState = newState
	return nil
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

type TcpEndpoint struct {
	tcb       TransmissionControlBlock
	link      chan byte
	terminate chan bool
}

func (endpoint *TcpEndpoint) Listen() {
	for b := range endpoint.link {
		fmt.Printf("Received byte %c\n", b)
		if b == 'b' {
			break
		}
	}

	endpoint.terminate <- true
}

func (endpoint *TcpEndpoint) receive(segment TcpSegment, segmentLength uint16) {
	controlBits := segment.Header.controlBits
	var err error = nil
	switch controlBits {
	case ControlBitMaskCongestionWindowReduced:
	case ControlBitMaskECNEcho:
	case ControlBitMaskUrgentPointer:
	case ControlBitMaskAck:
		err = endpoint.processAck(segment)
	case ControlBitMaskPushFunction:
	case ControlBitMaskReset:
	case ControlBitMaskSyn:
		err = endpoint.processRcv(segment, segmentLength)
	case ControlBitMaskFin:
	}

	if err != nil {
		panic("error occurred when receiving segment")
	}
}

/*
After an endpoint has sent a segment, it should receive an ACK in response.  This ACK must be validated.
The incoming segment header's ACK num must lie in the range of the TCB's send (UnAck, Next].
This is because the ACK num represents the sequence num which the receiver expects to receive next.
Modulo arithmetic must be handled with care as the unsigned sequence num wraps to 0 upon reaching 2^32.

(U)NACK   (A)CK    (N)EXT; six different letter combinations

+-----U------A-------N------+
+-----N------U-------A------+
+-----A------N-------U------+

The above shows the different positions the numbers can lie at due to modulo wrap.
*/
func (endpoint *TcpEndpoint) processAck(segment TcpSegment) error {
	sendUnAck := endpoint.tcb.SendUnAck
	ackNum := segment.Header.ackNumber
	sendNext := endpoint.tcb.SendNext

	if sendUnAck < sendNext {
		if ackNum <= sendUnAck {
			return errors.New("send unack seq num is before the window")
		}
		if ackNum > sendNext {
			return errors.New("send unack seq num is after the window")
		}
	} else if ackNum > sendNext && ackNum <= sendUnAck {
		return errors.New("send unack seq num is outside the window")
	}

	return nil
}

/*
An endpoint can receive a TCP segment.  This segment should be validated.

Rcv.Next (N)   Seg.Seq (S)   Seg.Seq + Seg.Len (S+L)   Rcv.Next + Rcv.Win (N+W)

+----N-----------S-------------------S+L-----------------------N+W  VALID
+----S-----------N-------------------S+L-----------------------N+W  VALID
+----N-----------S-------------------N+W-----------------------S+L  VALID
+----S-----------S+L-------------------N-----------------------N+W  INVALID (no seq num overlap with rcv window)
*/
func (endpoint *TcpEndpoint) processRcv(segment TcpSegment, segmentLength uint16) error {
	receiveNext := endpoint.tcb.ReceiveNext
	receiveWindow := endpoint.tcb.ReceiveWindow
	segmentSeqNum := segment.Header.seqNumber

	rl := endpoint.tcb.ReceiveNext
	rr := rl + receiveWindow - 1
	sl := segment.Header.seqNumber
	sr := sl + uint32(segmentLength)

	if segmentLength == 0 {
		if receiveWindow == 0 && segmentSeqNum != receiveNext {
			return errors.New("0 seg.len and rcv.win, but seg.seqNum != rcv.next")
		}
		if receiveWindow > 0 {
			if rl < rr {
				if sl < rl || sl >= rr {
					return errors.New("seg.len == 0 && rcv.win > 0, but seg.seq outside of window")
				}
			} else {
				if sl >= rr && sl < rl {
					return errors.New("seg.len == 0 && rcv.win > 0, but seg.seq outside of window")
				}
			}
		}
	} else if receiveWindow == 0 {
		return errors.New("segment length should not be > 0 when receive window is 0")
	} else if rl < rr {
		if sl < rl && sr < rl {
			return errors.New("segment sequence window is before receive next window")
		}
		if sl >= rr && sr >= rr {
			return errors.New("segment sequence window is after receive next window")
		}
	} else {
		if sl >= rr && sl < rl && sr >= rr && sr < rl {
			return errors.New("segment sequence window is outside receive next window")
		}
	}

	return nil
}

func (endpoint *TcpEndpoint) Send() {
	for {
		letter := byte(rand.Int()%26 + 'a')
		endpoint.link <- letter
		if letter == 'b' {
			break
		}
	}

	endpoint.terminate <- true
}

func main() {
	link := make(chan byte, 1_000)
	terminate := make(chan bool)
	server := TcpEndpoint{link: link, terminate: terminate}
	client := TcpEndpoint{link: link, terminate: terminate}

	go server.Listen()
	go client.Send()

	<-terminate
	<-terminate
	fmt.Println("terminated")
}
