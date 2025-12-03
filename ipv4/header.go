package ipv4

import "unsafe"

const (
	fragmentOffsetBitLength               = 13
	fragmentOffsetBitMask                 = 0b0001_1111_1111_1111
	FlagDontFragment               uint16 = 0b010 << fragmentOffsetBitLength
	flagMoreFragments              uint16 = 0b001 << fragmentOffsetBitLength
	totalLengthBitLength                  = 16
	headerByteLengthWithoutOptions        = 20

	HeaderOptionTypeCopiedFlagMask = 0b1000_0000
	HeaderOptionTypeClassMask      = 0b0110_0000
	HeaderOptionTypeNumberMask     = 0b0001_1111

	HeaderOptionEndOfOptions        = 0b0000_0000
	HeaderOptionNoOperation         = 0b0000_0000
	HeaderOptionSecurity            = 0b0000_0000
	HeaderOptionLooseSourceRouting  = 0b0000_0000
	HeaderOptionStrictSourceRouting = 0b0000_0000
	HeaderOptionRecordRoute         = 0b0000_0000
	HeaderOptionStreamId            = 0b0000_0000
	HeaderOptionInternetTimestamp   = 0b0000_0000
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
	ihlByteCount := h.GetIHLInBytes()
	optionsCount := uint8(ihlByteCount - headerByteLengthWithoutOptions)
	if optionsCount == 0 {
		return make([]uint8, 0)
	}

	optionTypeIndex := uint8(0)
	result := make([]uint8, 0, optionsCount)
	for optionTypeIndex < optionsCount {
		optionType := h.Options[optionTypeIndex]

		if optionType == 0 {
			break
		}

		if optionType&HeaderOptionTypeCopiedFlagMask == 1 {
			number := optionType & HeaderOptionTypeNumberMask

			if number >= 2 {
				optionLength := h.Options[optionTypeIndex+1]
				result = append(result, h.Options[optionTypeIndex:optionLength]...)
				optionTypeIndex += optionLength
			} else {
				result = append(result, optionType)
				optionTypeIndex++
			}
		}
	}

	return result
}

func (h *Header) setFlagState(flag uint16, enable bool) {
	if enable {
		h.FlagsAndFragmentOffsetEightBytes |= flag
	} else {
		h.FlagsAndFragmentOffsetEightBytes &^= flag
	}
}

func onesComplementSum(a, b uint16) uint16 {
	var result uint32 = uint32(a) + uint32(b)
	var carry uint16 = uint16(result>>16) & 1

	return uint16(result&0xFFFF) + carry
}
