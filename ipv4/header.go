package ipv4

import (
	"errors"
	"math"
	"unsafe"
)

const (
	fragmentOffsetBitLength               = 13
	fragmentOffsetBitMask                 = 0b0001_1111_1111_1111
	FlagDontFragment               uint16 = 0b010 << fragmentOffsetBitLength
	flagMoreFragments              uint16 = 0b001 << fragmentOffsetBitLength
	totalLengthBitLength                  = 16
	headerByteLengthWithoutOptions        = 20

	OptionMaskCopiedFlag = 0b1000_0000
	OptionMaskClass      = 0b0110_0000
	OptionMaskNumber     = 0b0001_1111

	OptionTypeEndOfOptions        = 0b0000_0000
	OptionTypeNoOperation         = 0b0000_0001
	OptionTypeSecurity            = 0b1000_0010
	OptionTypeLooseSourceRouting  = 0b1000_0011
	OptionTypeStrictSourceRouting = 0b1000_1001
	OptionTypeRecordRoute         = 0b0000_0111
	OptionTypeStreamId            = 0b1000_1000
	OptionTypeInternetTimestamp   = 0b0100_0100
)

type OptionsBuilder struct {
	containsRouteOption bool
	options             []uint8
}

func (ob *OptionsBuilder) AddRecordRoute(addressCount uint8) error {
	if ob.containsRouteOption {
		return errors.New("route option already included")
	}
	ob.containsRouteOption = true

	option, err := createRouteOption(OptionTypeRecordRoute, addressCount)
	if err != nil {
		return err
	}

	if len(ob.options)+len(option) > math.MaxUint8 {
		return errors.New("adding this option will exeed allowed options size")
	}

	ob.options = append(ob.options, option...)
	return nil
}

func createRouteOption(optionType uint8, addressCount uint8) ([]uint8, error) {
	const routeMetadataByteCount = 3

	length := routeMetadataByteCount + addressCount*4
	if addressCount < 1 {
		return nil, errors.New("number of addresses must be at least one")
	}

	const pointerStartValue uint8 = routeMetadataByteCount + 1
	option := make([]uint8, length)
	option[0] = optionType
	option[1] = uint8(length)
	option[2] = pointerStartValue

	return option, nil
}

func (ob *OptionsBuilder) AddLooseSourceRoute(addresses []Address) error {
	return ob.addSourceRoute(OptionTypeLooseSourceRouting, addresses)
}

func (ob *OptionsBuilder) AddStrictSourceRoute(addresses []Address) error {
	return ob.addSourceRoute(OptionTypeStrictSourceRouting, addresses)
}

func (ob *OptionsBuilder) addSourceRoute(optionType uint8, addresses []Address) error {
	if len(addresses) > math.MaxUint8 {
		return errors.New("too many addresses")
	}
	if ob.containsRouteOption {
		return errors.New("route option already included")
	}
	ob.containsRouteOption = true

	option, err := createRouteOption(optionType, uint8(len(addresses)))
	if err != nil {
		return err
	}

	if len(ob.options)+len(option) > math.MaxUint8 {
		return errors.New("adding this option will exeed allowed options size")
	}

	copy(option[3:], unsafe.Slice((*uint8)(unsafe.Pointer(&addresses[0])), len(addresses)*4))
	ob.options = append(ob.options, option...)

	return nil
}

func (ob *OptionsBuilder) Build() []uint8 {
	result := padToFourByteBoundary(ob.options)
	ob.options = []uint8{}

	return result
}

type HeaderOptionManager struct {
	header  *Header
	options map[uint8][]uint8
}

func MakeHeaderOptionManager(header *Header) HeaderOptionManager {
	optionMap := make(map[uint8][]uint8)
	for option := range header.GetOptions() {
		optionMap[option[0]] = option
	}

	return HeaderOptionManager{header, optionMap}
}

func (om *HeaderOptionManager) HasOption(optionType uint8) bool {
	_, exists := om.options[optionType]
	return exists
}

func (om *HeaderOptionManager) HasMultipleRouteOptions() bool {
	_, rr := om.options[OptionTypeRecordRoute]
	_, lr := om.options[OptionTypeLooseSourceRouting]
	_, sr := om.options[OptionTypeStrictSourceRouting]

	count := 0
	for _, exists := range []bool{rr, lr, sr} {
		if exists {
			count++
		}
	}

	return count > 1
}

func (hom *HeaderOptionManager) UpdateHeaderWithRecordRouteOption() {
	if option, exists := hom.options[OptionTypeRecordRoute]; exists {
		hom.updateRouteOption(option, false)
	}
}

func (hom *HeaderOptionManager) UpdateHeaderWithSourceRouteOption() bool {
	if option, exists := hom.getSourceRouteOption(); exists {
		hom.updateRouteOption(option, true)
		return true
	}

	return false
}

func (hom *HeaderOptionManager) updateRouteOption(option []uint8, isSourceRoute bool) {
	length, pointer := option[0], option[1]
	if pointer > length {
		return
	}

	// map from RFC logical address (e.g. min value of 4) to data array reality (e.g. actually 3)
	address := (*Address)(unsafe.Pointer(&option[pointer-1]))
	if isSourceRoute {
		hom.header.DestinationAddress = *address
	}
	*address = hom.header.SourceAddress
	pointer += 4
}

func (hom *HeaderOptionManager) getSourceRouteOption() ([]uint8, bool) {
	if option, exists := hom.options[OptionTypeLooseSourceRouting]; exists {
		return option, true
	}

	if option, exists := hom.options[OptionTypeStrictSourceRouting]; exists {
		return option, true
	}

	return nil, false
}

func (om *HeaderOptionManager) GetStrictSourceAddress() (Address, bool) {
	option, exists := om.options[OptionTypeStrictSourceRouting]
	if !exists {
		return Address(0), false
	}

	return *(*uint32)(unsafe.Pointer(&option[option[2]])), true
}

func (om *HeaderOptionManager) GetLooseSourceAddress() (Address, bool) {
	option, exists := om.options[OptionTypeLooseSourceRouting]
	if !exists {
		return Address(0), false
	}

	return *(*uint32)(unsafe.Pointer(&option[option[2]])), true
}

func (om *HeaderOptionManager) AllOptionsAreValid() (bool, error) {
	return true, nil
}

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
	if h.isPartOfFragmentedDatagram() {
		return h.Options
	}

	optionsCount := uint8(h.GetIHLInBytes() - headerByteLengthWithoutOptions)
	resultOptions := make([]uint8, 0, optionsCount)

	for option := range h.GetOptions() {
		optionType := option[0]
		if optionType&OptionMaskCopiedFlag == 1 {
			optionNumber := optionType & OptionMaskNumber
			if optionNumber >= 2 {
				optionLength := option[1]
				resultOptions = append(resultOptions, option[:optionLength]...)
			} else {
				resultOptions = append(resultOptions, option[:1]...)
			}
		}
	}

	return padToFourByteBoundary(resultOptions)
}

func (h *Header) GetOptions() <-chan []uint8 {
	results := make(chan []uint8)

	go func() {
		defer close(results)

		optionsCount := uint8(h.GetIHLInBytes() - headerByteLengthWithoutOptions)
		if optionsCount == 0 {
			return
		}

		optionIndex := uint8(0)
		for optionIndex < optionsCount {
			option := h.Options[optionIndex]

			if option == 0 {
				return
			}
			if option == 1 {
				optionIndex++
				continue
			}

			number := option & OptionMaskNumber
			if number >= 2 {
				optionLength := h.Options[optionIndex+1]
				results <- h.Options[optionIndex:optionLength]
				optionIndex += optionLength
			} else {
				results <- h.Options[optionIndex : optionIndex+1]
				optionIndex++
			}
		}
	}()

	return results
}

func (h *Header) setFlagState(flag uint16, enable bool) {
	if enable {
		h.FlagsAndFragmentOffsetEightBytes |= flag
	} else {
		h.FlagsAndFragmentOffsetEightBytes &^= flag
	}
}

func (h *Header) isPartOfFragmentedDatagram() bool {
	return h.MoreFragments() || h.GetFragmentOffset() != 0
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
