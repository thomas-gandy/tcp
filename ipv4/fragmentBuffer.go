package ipv4

import "time"

type FragmentBufferId struct {
	identifier  uint16
	source      uint32
	destination uint32
}

func makeFragmentBufferId(header *Header) FragmentBufferId {
	return FragmentBufferId{
		identifier:  header.Identification,
		source:      header.SourceAddress,
		destination: header.DestinationAddress,
	}
}

type FragmentBuffer struct {
	headerBuffer          Header
	dataBuffer            [1 << totalLengthBitLength]byte
	fragmentBlockBitTable [(1 << (fragmentOffsetBitLength - 3)) + 1]byte
	totalDataLength       uint16
	startUnixMs           int64
}

func newFragmentBuffer() *FragmentBuffer {
	return &FragmentBuffer{
		startUnixMs: time.Now().UnixMilli(),
	}
}

func (fb *FragmentBuffer) Insert(fragment *Datagram) *Datagram {
	header := fragment.Header
	fragmentOffsetEightBytes := header.GetFragmentOffset()
	if !header.MoreFragments() && fragmentOffsetEightBytes == 0 {
		return fragment
	}
	if header.GetFragmentOffset() == 0 {
		fb.headerBuffer = fragment.Header
	}

	fb.insertFragmentIntoDataBuffer(fragment)
	fb.setBlockBitsForFragment(fragment)

	if !header.MoreFragments() {
		fb.totalDataLength = fragmentOffsetEightBytes*8 + header.TotalLengthBytes - header.GetIHLInBytes()
	}

	if fb.totalDataLength != 0 {
		dataComplete := fb.fragmentBlocksComplete(fb.totalDataLength)
		if dataComplete {
			return &Datagram{
				Header: fb.headerBuffer,
				Data:   fb.dataBuffer[:fb.totalDataLength],
			}
		}
	}

	return nil
}

func (fb *FragmentBuffer) insertFragmentIntoDataBuffer(fragment *Datagram) {
	byteOffset := fragment.Header.GetFragmentOffset() * 8
	dataByteLength := fragment.Header.TotalLengthBytes - fragment.Header.GetIHLInBytes()
	copy(fb.dataBuffer[byteOffset:byteOffset+dataByteLength], fragment.Data)
}

func (fb *FragmentBuffer) fragmentBlocksComplete(totalLength uint16) bool {
	endBitIndex := (totalLength + 7) / 8
	fullBitsToCheck := endBitIndex / 8
	for i := range fullBitsToCheck {
		if fb.fragmentBlockBitTable[i] != 0xFF {
			return false
		}
	}
	mask := makeBitMaskForBitIndicesInByte(0, endBitIndex%8)

	return fb.fragmentBlockBitTable[fullBitsToCheck]&mask == mask
}

func (fb *FragmentBuffer) setBlockBitsForFragment(fragment *Datagram) {
	startBitIndex := fragment.Header.GetFragmentOffset()
	hl := fragment.Header.GetIHLInBytes()
	dataByteLength := fragment.Header.TotalLengthBytes - hl

	// It is not possible for a block bit to be set by e.g. the last four bytes of one fragment
	// and then for it to be set again by the first four bytes of a second fragment.  Only the last
	// fragment is able to have data on a non-eight byte boundary.
	//
	// You may start out thinking you want dataByteLength [0, 8] to collapse to the same bit.
	// However, 0 is an edge case; [8, 16] should collapse to different bits.  You really want
	// [1, 8] to collapse to the same bit.  However, dividing 1 by 8 and 8 by 8 will give different
	// bits.  This could make the algorithm incorrectly think a fragment has been ingested if the
	// fragment before it came in first.
	//
	// Doing nothing or subtracting 7 means [0, 7] collapse to the same bit.  Subtracting 1 or
	// adding 7 means [1, 8] collapse to the same bit.  The spec expects dataByteLength to be a
	// length, so [1, 8] should be collapsed to the same byte.  The spec states an addition of 7.
	// If -1 is used you must assert dataByteLength > 0 and check bits [0, FO + (TL-IHL*4-1)/8].
	// If +7 is used, the buffer must contain 1 extra byte and check bits [0, FO + (TL-IHL*4+7)/8].
	//
	// If using -1 you risk an overflow when using uint16s if dataByteLength is 0 and it is the
	// first fragment.
	endBitIndex := startBitIndex + (dataByteLength+7)/8

	tableStartByteIndex := startBitIndex / 8
	tableEndByteIndex := endBitIndex / 8

	if tableStartByteIndex == tableEndByteIndex {
		mask := makeBitMaskForBitIndicesInByte(startBitIndex%8, endBitIndex%8)
		fb.fragmentBlockBitTable[tableStartByteIndex] |= mask
	} else {
		mask := makeBitMaskForBitIndicesInByte(startBitIndex%8, 7)
		fb.fragmentBlockBitTable[tableStartByteIndex] |= mask

		for byteIndex := tableStartByteIndex + 1; byteIndex < tableEndByteIndex; byteIndex++ {
			fb.fragmentBlockBitTable[byteIndex] |= 0xFF
		}

		mask = makeBitMaskForBitIndicesInByte(0, endBitIndex%8)
		fb.fragmentBlockBitTable[tableEndByteIndex] |= mask
	}
}

func makeBitMaskForBitIndicesInByte(si, ei uint16) byte {
	return uint8(int8(-0b1000_0000)>>int8(ei-si)) >> uint(si)
}
