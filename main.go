package main

const (
	OptionEndOfList      = 0 // used at end of all options (additional padding could occur after last option)
	OptionNoOperation    = 1 // can be used to align a subsequent option on a certain word boundary (not necessarily)
	OptionMaxSegmentSize = 2 // maximum segment size the sender can receive
)

const (
	SendStateUnAck = iota
	SendStateNext
	SendStateWindow
	SendStateUrgentPointer
	SendStateSegSeqNumForLastWinUpdate
	SendStateSegAckNumForLastWinUpdate
	SendStateInitialSendSeqNum
)

const (
	ReceiveStateNext = iota
	ReceiveStateReceiveWin
	ReceiveStateReceiveUrgentPointer
	ReceiveStateInitialReceiveSeqNum
)

type TransmissionControlBlock struct {
	LocalIP, DestinationIP     uint32
	LocalPort, DestinationPort uint16
	ReceiveBuffer, SendBuffer  []byte
	RetransmitQueue            []byte
	CurrentSegment             *TcpSegment
}

type TcpHeader struct {
	sourcePort, destinationPort uint16

	/*
		Sequence number of first octet (byte) in segment, or initial sequence number when SYN bit set
		Expected next sequence number for sender to receive when ACK bit set
	*/
	seqNumber, ackNumber uint32

	/*
		Number of 32 bit (4 byte) words in header; first four bits are data offset, the rest are reserved.
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

func main() {

}
