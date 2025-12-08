package ipv4

import "unsafe"

const (
	fragmentOffsetBitLength               = 13
	fragmentOffsetBitMask                 = 0b0001_1111_1111_1111
	FlagDontFragment               uint16 = 0b010 << fragmentOffsetBitLength
	flagMoreFragments              uint16 = 0b001 << fragmentOffsetBitLength
	totalLengthBitLength                  = 16
	headerByteLengthWithoutOptions        = 20

	MaskHeaderOptionCopiedFlag = 0b1000_0000
	MaskHeaderOptionClass      = 0b0110_0000
	MaskHeaderOptionNumber     = 0b0001_1111

	HeaderOptionTypeEndOfOptions        = 0b0000_0000
	HeaderOptionTypeNoOperation         = 0b0000_0001
	HeaderOptionTypeSecurity            = 0b1000_0010
	HeaderOptionTypeLooseSourceRouting  = 0b1000_0011
	HeaderOptionTypeStrictSourceRouting = 0b1000_1001
	HeaderOptionTypeRecordRoute         = 0b0000_0111
	HeaderOptionTypeStreamId            = 0b1000_1000
	HeaderOptionTypeInternetTimestamp   = 0b0100_0100
)

// Useful to remember header order: VITT IFF TPH SDO
// Something interesting to note is that only 13-bytes are available to the fragment offset, and
// 16-bytes available to the total offset, but because a single unit of a fragment is measured in
// terms of 8-bytes (2^3), it actually can specify an upper exclusive bound of 2^16.
type Header struct {
	VersionAndIHLFourBytes           uint8 // version (4-bits) and IHL (4-bits) (units of 4-bytes)
	TypeOfService                    uint8
	TotalLengthBytes                 uint16 // units of 1-byte
	Identification                   uint16
	FlagsAndFragmentOffsetEightBytes uint16 // flags (3-bits) and fragment offset (13-bits) (units of 8-bytes)
	TimeToLiveSecs                   uint8
	Protocol                         uint8
	HeaderChecksum                   uint16
	SourceAddress                    uint32
	DestinationAddress               uint32
	Options                          []uint8
}

func (h *Header) SetVersion(version uint8) {
	h.VersionAndIHLFourBytes |= version << 4
}

func (h *Header) SetIHL(dwords uint8) {
	h.VersionAndIHLFourBytes |= dwords
}

func (h *Header) GetIHL() uint8 {
	return h.VersionAndIHLFourBytes & 0b0000_1111
}

func (h *Header) GetIHLInBytes() uint16 {
	return uint16(h.VersionAndIHLFourBytes&0b0000_1111) * 4
}

func (h *Header) MayFragment() bool {
	return h.FlagsAndFragmentOffsetEightBytes&FlagDontFragment == 0
}

func (h *Header) SetMayFragment(allowFragmentation bool) {
	h.setFlagState(FlagDontFragment, !allowFragmentation)
}

func (h *Header) MoreFragments() bool {
	return h.FlagsAndFragmentOffsetEightBytes&flagMoreFragments != 0
}

func (h *Header) SetMoreFragments(moreFragments bool) {
	h.setFlagState(flagMoreFragments, moreFragments)
}

func (h *Header) SetFragmentOffset(offset uint16) {
	h.FlagsAndFragmentOffsetEightBytes &^= fragmentOffsetBitMask
	h.FlagsAndFragmentOffsetEightBytes |= offset & fragmentOffsetBitMask
}

func (h *Header) GetFragmentOffset() uint16 {
	return h.FlagsAndFragmentOffsetEightBytes & fragmentOffsetBitMask
}

func (h *Header) GetChecksum() uint16 {
	data := (*uint16)(unsafe.Pointer(h))
	chunkCount := int(h.GetIHL()) * 2 // 16-bit chunk sizes
	var sum uint16 = 0

	for range 5 {
		sum = onesComplementSum(sum, *data)
		data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16))
	}

	data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16)) // skip checksum itself

	for i := 6; i < chunkCount; i++ {
		sum = onesComplementSum(sum, *data)
		data = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + 16))
	}

	return ^sum
}

func (h *Header) GetFragmentCopyableOptionData() []uint8 {
	optionsCount := uint8(h.GetIHLInBytes() - headerByteLengthWithoutOptions)
	if optionsCount == 0 {
		return make([]uint8, 0)
	}

	optionIndex := uint8(0)
	resultOptions := make([]uint8, 0, optionsCount)
	for optionIndex < optionsCount {
		option := h.Options[optionIndex]

		if option == 0 {
			break
		}
		if option == 1 {
			optionIndex++
			continue
		}

		if option&MaskHeaderOptionCopiedFlag == 1 {
			number := option & MaskHeaderOptionNumber

			if number >= 2 {
				optionLength := h.Options[optionIndex+1]
				resultOptions = append(resultOptions, h.Options[optionIndex:optionLength]...)
				optionIndex += optionLength
			} else {
				resultOptions = append(resultOptions, option)
				optionIndex++
			}
		}
	}

	return padToFourByteBoundary(resultOptions)
}

func (h *Header) setFlagState(flag uint16, enable bool) {
	if enable {
		h.FlagsAndFragmentOffsetEightBytes |= flag
	} else {
		h.FlagsAndFragmentOffsetEightBytes &^= flag
	}
}

func padToFourByteBoundary(options []uint8) []uint8 {
	remainder := len(options) % 4
	if remainder > 0 {
		paddingBytesNeeded := 4 - remainder
		for range paddingBytesNeeded {
			options = append(options, 0)
		}
	}

	return options
}

func onesComplementSum(a, b uint16) uint16 {
	var result uint32 = uint32(a) + uint32(b)
	var carry uint16 = uint16(result>>16) & 1

	return uint16(result&0xFFFF) + carry
}
