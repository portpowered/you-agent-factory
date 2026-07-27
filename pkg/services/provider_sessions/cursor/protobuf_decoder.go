package cursor

import (
	"encoding/binary"
	"fmt"
)

// decodeVarint decodes a protobuf varint and returns the value and number of bytes consumed
func decodeVarint(data []byte) (uint64, int) {
	var result uint64
	var shift uint
	bytesRead := 0

	for i, b := range data {
		if i >= 10 { // Varints can be at most 10 bytes
			return 0, 0
		}

		result |= uint64(b&0x7F) << shift
		bytesRead++

		if (b & 0x80) == 0 {
			break
		}

		shift += 7
	}

	return result, bytesRead
}

// extractProtobufFields extracts all fields from protobuf data and returns them as a map
// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session protobuf decoding stays grouped until extraction refactor.
// This is a simplified decoder that focuses on extracting readable content
func extractProtobufFields(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	offset := 0
	fieldCount := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		// Read tag
		tag := data[offset]
		offset++

		wireType := tag & 0x07
		fieldNum := tag >> 3

		fieldKey := fmt.Sprintf("field_%d", fieldNum)
		fieldCount++

		switch wireType {
		case 0: // Varint
			value, bytesRead := decodeVarint(data[offset:])
			if bytesRead == 0 {
				return result, fmt.Errorf("invalid varint at offset %d", offset)
			}
			result[fieldKey] = value
			offset += bytesRead

		case 1: // 64-bit (fixed64)
			if offset+8 > len(data) {
				return result, fmt.Errorf("not enough data for 64-bit at offset %d", offset)
			}
			value := binary.LittleEndian.Uint64(data[offset : offset+8])
			result[fieldKey] = value
			offset += 8

		case 2: // Length-delimited (string, bytes, embedded message)
			length, lengthBytes := decodeVarint(data[offset:])
			if lengthBytes == 0 {
				return result, fmt.Errorf("invalid length varint at offset %d", offset)
			}
			offset += lengthBytes

			if offset+int(length) > len(data) {
				return result, fmt.Errorf("not enough data for length-delimited field at offset %d", offset)
			}

			fieldData := data[offset : offset+int(length)]
			offset += int(length)

			// Try to decode as string
			if isReadableText(string(fieldData)) {
				result[fieldKey] = string(fieldData)
			} else {
				// Try to extract JSON
				if jsonBytes, found := extractJSONFromBinary(fieldData); found {
					result[fieldKey] = string(jsonBytes)
				} else {
					// Try to decode nested protobuf
					if nestedFields, err := extractProtobufFields(fieldData); err == nil && len(nestedFields) > 0 {
						result[fieldKey] = nestedFields
					} else {
						// Store as hex for debugging
						result[fieldKey] = fmt.Sprintf("[binary: %d bytes]", len(fieldData))
					}
				}
			}

		case 5: // 32-bit (fixed32)
			if offset+4 > len(data) {
				return result, fmt.Errorf("not enough data for 32-bit at offset %d", offset)
			}
			value := binary.LittleEndian.Uint32(data[offset : offset+4])
			result[fieldKey] = value
			offset += 4

		default:
			return result, fmt.Errorf("unknown wire type %d at offset %d", wireType, offset-1)
		}

		// Safety check - don't process more than 100 fields
		if fieldCount > 100 {
			break
		}
	}

	return result, nil
}

func tryProtobufDecode(data []byte) (map[string]interface{}, bool) {
	// Check if it looks like protobuf (starts with valid tag bytes)
	if len(data) == 0 {
		return nil, false
	}

	firstByte := data[0]
	wireType := firstByte & 0x07

	// Valid protobuf should have reasonable field numbers and wire types (0-5)
	// Field numbers are varints, but we check the first byte only
	if wireType > 5 {
		return nil, false
	}

	// Try to decode
	fields, err := extractProtobufFields(data)
	if err != nil {
		return nil, false
	}

	if len(fields) == 0 {
		return nil, false
	}

	return fields, true
}
