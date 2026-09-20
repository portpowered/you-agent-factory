package runner

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"strconv"
)

type corpusV2ISOBox struct {
	typ     string
	payload []byte
}

type corpusV2MP4Track struct {
	handler        string
	codec          string
	width          int
	height         int
	timescale      uint32
	duration       uint64
	sampleCount    uint64
	sampleDuration uint64
}

func parseCorpusV2MP4Metadata(data []byte) (CorpusV2StreamMetadata, error) {
	root, err := parseCorpusV2ISOBoxes(data)
	if err != nil {
		return CorpusV2StreamMetadata{}, err
	}
	moov, err := corpusV2SingleBoxPayload(root, "moov")
	if err != nil {
		return CorpusV2StreamMetadata{}, err
	}
	tracks, err := parseCorpusV2MP4Tracks(moov)
	if err != nil {
		return CorpusV2StreamMetadata{}, err
	}
	var video *corpusV2MP4Track
	for index := range tracks {
		track := &tracks[index]
		if track.handler != "vide" {
			continue
		}
		if video != nil || (track.codec != "avc1" && track.codec != "avc3") {
			return CorpusV2StreamMetadata{}, fmt.Errorf("expected one H.264 video track")
		}
		video = track
	}
	if video == nil {
		return CorpusV2StreamMetadata{}, fmt.Errorf("MP4 has no H.264 video track")
	}
	if video.timescale == 0 || video.duration == 0 || video.sampleCount == 0 || video.sampleDuration == 0 {
		return CorpusV2StreamMetadata{}, fmt.Errorf("video timing table is incomplete")
	}
	if video.width <= 0 || video.height <= 0 || video.sampleCount > math.MaxInt64 {
		return CorpusV2StreamMetadata{}, fmt.Errorf("video dimensions or frame count exceed supported range")
	}
	durationMillis, err := corpusV2RoundedMillis(video.duration, uint64(video.timescale))
	if err != nil {
		return CorpusV2StreamMetadata{}, err
	}
	frameRateNumerator := new(big.Int).Mul(new(big.Int).SetUint64(video.sampleCount), new(big.Int).SetUint64(uint64(video.timescale)))
	frameRateDenominator := new(big.Int).SetUint64(video.sampleDuration)
	if !frameRateDenominator.IsUint64() || frameRateDenominator.Sign() <= 0 || !frameRateNumerator.IsUint64() {
		return CorpusV2StreamMetadata{}, fmt.Errorf("frame-rate values exceed supported range")
	}
	frameRate := corpusV2RationalString(frameRateNumerator.Uint64(), frameRateDenominator.Uint64())
	metadata := CorpusV2StreamMetadata{
		Codec:               video.codec,
		Width:               video.width,
		Height:              video.height,
		FrameRate:           frameRate,
		DurationMillis:      durationMillis,
		DurationSeconds:     float64(video.duration) / float64(video.timescale),
		Frames:              int64(video.sampleCount),
		durationNumerator:   video.duration,
		durationDenominator: uint64(video.timescale),
	}
	if err := validateCorpusV2Metadata(metadata); err != nil {
		return CorpusV2StreamMetadata{}, err
	}
	return metadata, nil
}

func corpusV2RoundedMillis(numerator, denominator uint64) (int64, error) {
	if denominator == 0 {
		return 0, fmt.Errorf("duration denominator is zero")
	}
	scaled := new(big.Int).Mul(new(big.Int).SetUint64(numerator), big.NewInt(1000))
	scaled.Add(scaled, new(big.Int).SetUint64(denominator/2))
	scaled.Quo(scaled, new(big.Int).SetUint64(denominator))
	if !scaled.IsInt64() || scaled.Sign() <= 0 {
		return 0, fmt.Errorf("duration exceeds supported range")
	}
	return scaled.Int64(), nil
}

func parseCorpusV2MP4Tracks(data []byte) ([]corpusV2MP4Track, error) {
	boxes, err := parseCorpusV2ISOBoxes(data)
	if err != nil {
		return nil, err
	}
	tracks := corpusV2Boxes(boxes, "trak")
	if len(tracks) == 0 {
		return nil, fmt.Errorf("MP4 has no tracks")
	}
	parsed := make([]corpusV2MP4Track, 0, len(tracks))
	for index, box := range tracks {
		track, err := parseCorpusV2MP4Track(box.payload)
		if err != nil {
			return nil, fmt.Errorf("track %d: %w", index, err)
		}
		parsed = append(parsed, track)
	}
	return parsed, nil
}

func parseCorpusV2MP4Track(data []byte) (corpusV2MP4Track, error) {
	trak, err := parseCorpusV2ISOBoxes(data)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	tkhd, err := corpusV2SingleBoxPayload(trak, "tkhd")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	mdia, err := corpusV2SingleBoxPayload(trak, "mdia")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	width, height, err := corpusV2TrackDimensions(tkhd)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	media, err := parseCorpusV2ISOBoxes(mdia)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	mdhd, err := corpusV2SingleBoxPayload(media, "mdhd")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	handlerPayload, err := corpusV2SingleBoxPayload(media, "hdlr")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	handler, err := corpusV2TrackHandler(handlerPayload)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	minf, err := corpusV2SingleBoxPayload(media, "minf")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	minfBoxes, err := parseCorpusV2ISOBoxes(minf)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	stbl, err := corpusV2SingleBoxPayload(minfBoxes, "stbl")
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	sampleTable, err := parseCorpusV2ISOBoxes(stbl)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	codec, err := corpusV2SampleDescription(corpusV2Boxes(sampleTable, "stsd"))
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	sampleCount, sampleDuration, err := corpusV2TimeToSample(corpusV2Boxes(sampleTable, "stts"))
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	timescale, duration, err := corpusV2MediaHeader(mdhd)
	if err != nil {
		return corpusV2MP4Track{}, err
	}
	return corpusV2MP4Track{handler: handler, codec: codec, width: width, height: height, timescale: timescale, duration: duration, sampleCount: sampleCount, sampleDuration: sampleDuration}, nil
}

func parseCorpusV2ISOBoxes(data []byte) ([]corpusV2ISOBox, error) {
	boxes := make([]corpusV2ISOBox, 0)
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, fmt.Errorf("truncated box header at byte %d", offset)
		}
		size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		headerSize := uint64(8)
		if size == 1 {
			if len(data)-offset < 16 {
				return nil, fmt.Errorf("truncated extended box size at byte %d", offset)
			}
			size = binary.BigEndian.Uint64(data[offset+8 : offset+16])
			headerSize = 16
		} else if size == 0 {
			size = uint64(len(data) - offset)
		}
		if size < headerSize || size > uint64(len(data)-offset) || size > uint64(math.MaxInt) {
			return nil, fmt.Errorf("invalid box size %d at byte %d", size, offset)
		}
		end := offset + int(size)
		boxes = append(boxes, corpusV2ISOBox{typ: string(data[offset+4 : offset+8]), payload: data[offset+int(headerSize) : end]})
		offset = end
	}
	return boxes, nil
}

func corpusV2Boxes(boxes []corpusV2ISOBox, typ string) []corpusV2ISOBox {
	matching := make([]corpusV2ISOBox, 0, 1)
	for _, box := range boxes {
		if box.typ == typ {
			matching = append(matching, box)
		}
	}
	return matching
}

func corpusV2SingleBoxPayload(boxes []corpusV2ISOBox, typ string) ([]byte, error) {
	matching := corpusV2Boxes(boxes, typ)
	if len(matching) != 1 {
		return nil, fmt.Errorf("expected exactly one %s box, found %d", typ, len(matching))
	}
	return matching[0].payload, nil
}

func corpusV2TrackDimensions(data []byte) (int, int, error) {
	if len(data) < 4 {
		return 0, 0, fmt.Errorf("truncated tkhd")
	}
	widthOffset := 0
	switch data[0] {
	case 0:
		widthOffset = 76
	case 1:
		widthOffset = 88
	default:
		return 0, 0, fmt.Errorf("unsupported tkhd version %d", data[0])
	}
	if len(data) < widthOffset+8 {
		return 0, 0, fmt.Errorf("truncated tkhd dimensions")
	}
	width := binary.BigEndian.Uint32(data[widthOffset : widthOffset+4])
	height := binary.BigEndian.Uint32(data[widthOffset+4 : widthOffset+8])
	if width&0xffff != 0 || height&0xffff != 0 || width>>16 > math.MaxInt32 || height>>16 > math.MaxInt32 {
		return 0, 0, fmt.Errorf("unsupported fractional track dimensions")
	}
	return int(width >> 16), int(height >> 16), nil
}

func corpusV2TrackHandler(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("truncated hdlr")
	}
	return string(data[8:12]), nil
}

func corpusV2MediaHeader(data []byte) (uint32, uint64, error) {
	if len(data) < 4 {
		return 0, 0, fmt.Errorf("truncated mdhd")
	}
	switch data[0] {
	case 0:
		if len(data) < 20 {
			return 0, 0, fmt.Errorf("truncated version 0 mdhd")
		}
		return binary.BigEndian.Uint32(data[12:16]), uint64(binary.BigEndian.Uint32(data[16:20])), nil
	case 1:
		if len(data) < 32 {
			return 0, 0, fmt.Errorf("truncated version 1 mdhd")
		}
		return binary.BigEndian.Uint32(data[20:24]), binary.BigEndian.Uint64(data[24:32]), nil
	default:
		return 0, 0, fmt.Errorf("unsupported mdhd version %d", data[0])
	}
}

func corpusV2SampleDescription(boxes []corpusV2ISOBox) (string, error) {
	if len(boxes) != 1 || len(boxes[0].payload) < 16 {
		return "", fmt.Errorf("expected one non-empty stsd")
	}
	data := boxes[0].payload
	entryCount := binary.BigEndian.Uint32(data[4:8])
	if entryCount != 1 {
		return "", fmt.Errorf("expected one sample description entry")
	}
	entrySize := uint64(binary.BigEndian.Uint32(data[8:12]))
	if entrySize < 8 || entrySize > uint64(len(data)-8) || string(data[12:16]) == "" {
		return "", fmt.Errorf("invalid sample description entry")
	}
	return string(data[12:16]), nil
}

func corpusV2TimeToSample(boxes []corpusV2ISOBox) (uint64, uint64, error) {
	if len(boxes) != 1 || len(boxes[0].payload) < 8 {
		return 0, 0, fmt.Errorf("expected one non-empty stts")
	}
	data := boxes[0].payload
	entryCount := binary.BigEndian.Uint32(data[4:8])
	entryBytes := uint64(entryCount) * 8
	if entryBytes > uint64(len(data)-8) {
		return 0, 0, fmt.Errorf("truncated stts entries")
	}
	var sampleCount, sampleDuration uint64
	for offset := 8; offset < 8+int(entryBytes); offset += 8 {
		count := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		duration := uint64(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if count > math.MaxUint64-sampleCount || (duration != 0 && count > (math.MaxUint64-sampleDuration)/duration) {
			return 0, 0, fmt.Errorf("stts values overflow")
		}
		sampleCount += count
		sampleDuration += count * duration
	}
	return sampleCount, sampleDuration, nil
}

func corpusV2RationalString(numerator, denominator uint64) string {
	divisor := corpusV2GreatestCommonDivisor(numerator, denominator)
	return strconv.FormatUint(numerator/divisor, 10) + "/" + strconv.FormatUint(denominator/divisor, 10)
}

func corpusV2GreatestCommonDivisor(first, second uint64) uint64 {
	for second != 0 {
		first, second = second, first%second
	}
	return first
}
