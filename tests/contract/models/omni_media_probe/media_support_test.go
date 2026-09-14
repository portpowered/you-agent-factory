package omni_media_probe

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

type isoBox struct {
	typ     string
	payload []byte
}

type mp4Track struct {
	handler        string
	codec          string
	width          int
	height         int
	timescale      uint32
	duration       uint64
	sampleCount    uint64
	sampleDuration uint64
}

func parseMP4Metadata(data []byte) (VideoMetadata, error) {
	root, err := parseISOBoxes(data)
	if err != nil {
		return VideoMetadata{}, err
	}
	if len(singleBoxes(root, "ftyp")) != 1 || len(singleBoxes(root, "moov")) != 1 {
		return VideoMetadata{}, fmt.Errorf("MP4 requires one ftyp and one moov box")
	}
	ftyp := singleBoxes(root, "ftyp")[0].payload
	if len(ftyp) < 8 || string(ftyp[:4]) != "isom" {
		return VideoMetadata{}, fmt.Errorf("unexpected MP4 major brand")
	}
	moov := singleBoxes(root, "moov")[0].payload
	tracks, err := parseMP4Tracks(moov)
	if err != nil {
		return VideoMetadata{}, err
	}
	var video *mp4Track
	audioCount := 0
	for index := range tracks {
		track := &tracks[index]
		switch track.handler {
		case "vide":
			if video != nil || track.codec != "avc1" {
				return VideoMetadata{}, fmt.Errorf("expected one H.264 video track")
			}
			video = track
		case "soun":
			if track.codec != "mp4a" {
				return VideoMetadata{}, fmt.Errorf("expected AAC audio track")
			}
			audioCount++
		}
	}
	if video == nil || audioCount != 1 {
		return VideoMetadata{}, fmt.Errorf("expected one video and one audio track")
	}
	if video.timescale == 0 || video.duration == 0 || video.sampleCount == 0 || video.sampleDuration == 0 {
		return VideoMetadata{}, fmt.Errorf("video timing table is incomplete")
	}
	if video.width <= 0 || video.height <= 0 || video.duration > math.MaxInt64/1000 || video.sampleCount > math.MaxInt64 {
		return VideoMetadata{}, fmt.Errorf("video metadata exceeds supported range")
	}
	durationMillisNumerator := video.duration * 1000
	if durationMillisNumerator%uint64(video.timescale) != 0 {
		return VideoMetadata{}, fmt.Errorf("video duration is not an exact millisecond")
	}
	durationMillis := int64(durationMillisNumerator / uint64(video.timescale))
	if video.sampleCount > math.MaxInt64/uint64(video.timescale) || video.sampleDuration > math.MaxInt64 {
		return VideoMetadata{}, fmt.Errorf("video frame-rate values exceed supported range")
	}
	frameRateNumerator := video.sampleCount * uint64(video.timescale)
	frameRate := rationalString(frameRateNumerator, video.sampleDuration)
	return VideoMetadata{
		Width:          video.width,
		Height:         video.height,
		DurationMillis: durationMillis,
		FrameRate:      frameRate,
		Frames:         int64(video.sampleCount),
	}, nil
}

func parseMP4Tracks(data []byte) ([]mp4Track, error) {
	moov, err := parseISOBoxes(data)
	if err != nil {
		return nil, err
	}
	tracks := singleBoxes(moov, "trak")
	if len(tracks) == 0 {
		return nil, fmt.Errorf("MP4 has no tracks")
	}
	parsed := make([]mp4Track, 0, len(tracks))
	for index, box := range tracks {
		track, err := parseMP4Track(box.payload)
		if err != nil {
			return nil, fmt.Errorf("track %d: %w", index, err)
		}
		parsed = append(parsed, track)
	}
	return parsed, nil
}

func parseMP4Track(data []byte) (mp4Track, error) {
	trak, err := parseISOBoxes(data)
	if err != nil {
		return mp4Track{}, err
	}
	tkhd, err := singleBoxPayload(trak, "tkhd")
	if err != nil {
		return mp4Track{}, err
	}
	mdia, err := singleBoxPayload(trak, "mdia")
	if err != nil {
		return mp4Track{}, err
	}
	width, height, err := parseTrackDimensions(tkhd)
	if err != nil {
		return mp4Track{}, err
	}
	media, err := parseISOBoxes(mdia)
	if err != nil {
		return mp4Track{}, err
	}
	mdhd, err := singleBoxPayload(media, "mdhd")
	if err != nil {
		return mp4Track{}, err
	}
	handlerPayload, err := singleBoxPayload(media, "hdlr")
	if err != nil {
		return mp4Track{}, err
	}
	handler, err := parseHandler(handlerPayload)
	if err != nil {
		return mp4Track{}, err
	}
	minf, err := singleBoxPayload(media, "minf")
	if err != nil {
		return mp4Track{}, err
	}
	minfBoxes, err := parseISOBoxes(minf)
	if err != nil {
		return mp4Track{}, err
	}
	stbl, err := singleBoxPayload(minfBoxes, "stbl")
	if err != nil {
		return mp4Track{}, err
	}
	sampleTable, err := parseISOBoxes(stbl)
	if err != nil {
		return mp4Track{}, err
	}
	codec, err := parseSampleDescription(singleBoxes(sampleTable, "stsd"))
	if err != nil {
		return mp4Track{}, err
	}
	sampleCount, sampleDuration, err := parseTimeToSample(singleBoxes(sampleTable, "stts"))
	if err != nil {
		return mp4Track{}, err
	}
	timescale, duration, err := parseMediaHeader(mdhd)
	if err != nil {
		return mp4Track{}, err
	}
	return mp4Track{
		handler:        handler,
		codec:          codec,
		width:          width,
		height:         height,
		timescale:      timescale,
		duration:       duration,
		sampleCount:    sampleCount,
		sampleDuration: sampleDuration,
	}, nil
}

func parseISOBoxes(data []byte) ([]isoBox, error) {
	boxes := make([]isoBox, 0)
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
		if size < headerSize || size > uint64(len(data)-offset) {
			return nil, fmt.Errorf("invalid box size %d at byte %d", size, offset)
		}
		end := offset + int(size)
		boxes = append(boxes, isoBox{typ: string(data[offset+4 : offset+8]), payload: data[offset+int(headerSize) : end]})
		offset = end
	}
	return boxes, nil
}

func singleBoxes(boxes []isoBox, typ string) []isoBox {
	matching := make([]isoBox, 0, 1)
	for _, box := range boxes {
		if box.typ == typ {
			matching = append(matching, box)
		}
	}
	return matching
}

func singleBoxPayload(boxes []isoBox, typ string) ([]byte, error) {
	matching := singleBoxes(boxes, typ)
	if len(matching) != 1 {
		return nil, fmt.Errorf("expected exactly one %s box, found %d", typ, len(matching))
	}
	return matching[0].payload, nil
}

func parseTrackDimensions(data []byte) (int, int, error) {
	if len(data) < 4 {
		return 0, 0, fmt.Errorf("truncated tkhd")
	}
	var widthOffset int
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

func parseHandler(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("truncated hdlr")
	}
	return string(data[8:12]), nil
}

func parseMediaHeader(data []byte) (uint32, uint64, error) {
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

func parseSampleDescription(boxes []isoBox) (string, error) {
	if len(boxes) != 1 || len(boxes[0].payload) < 8 {
		return "", fmt.Errorf("expected one non-empty stsd")
	}
	data := boxes[0].payload
	entryCount := binary.BigEndian.Uint32(data[4:8])
	if entryCount != 1 || len(data) < 16 {
		return "", fmt.Errorf("expected one sample description entry")
	}
	entrySize := uint64(binary.BigEndian.Uint32(data[8:12]))
	if entrySize < 8 || entrySize > uint64(len(data)-8) || string(data[12:16]) == "" {
		return "", fmt.Errorf("invalid sample description entry")
	}
	return string(data[12:16]), nil
}

func parseTimeToSample(boxes []isoBox) (uint64, uint64, error) {
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
		if count > math.MaxUint64-sampleCount || duration != 0 && count > (math.MaxUint64-sampleDuration)/duration {
			return 0, 0, fmt.Errorf("stts values overflow")
		}
		sampleCount += count
		sampleDuration += count * duration
	}
	return sampleCount, sampleDuration, nil
}

func rationalString(numerator, denominator uint64) string {
	divisor := greatestCommonDivisor(numerator, denominator)
	return strconv.FormatUint(numerator/divisor, 10) + "/" + strconv.FormatUint(denominator/divisor, 10)
}

func greatestCommonDivisor(first, second uint64) uint64 {
	for second != 0 {
		first, second = second, first%second
	}
	return first
}
