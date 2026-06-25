package repository

import (
	"encoding/base64"
	"strings"
)

func DNSZoneID(serverID, zoneName string) string {
	return encodeResourceID(serverID, zoneName)
}

func DNSRecordID(serverID, zoneName, recordType, recordName, recordValue string) string {
	return encodeResourceID(serverID, zoneName, recordType, recordName, recordValue)
}

func DecodeDNSZoneID(id string) (string, string, bool) {
	parts := strings.Split(id, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	serverID, ok := decodeResourceIDPart(parts[0])
	if !ok {
		return "", "", false
	}
	zoneName, ok := decodeResourceIDPart(parts[1])
	return serverID, zoneName, ok
}

func DecodeDNSRecordID(id string) (string, string, string, string, string, bool) {
	parts := strings.Split(id, ".")
	if len(parts) != 5 {
		return "", "", "", "", "", false
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value, ok := decodeResourceIDPart(part)
		if !ok {
			return "", "", "", "", "", false
		}
		values = append(values, value)
	}
	return values[0], values[1], values[2], values[3], values[4], true
}

func encodeResourceID(parts ...string) string {
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		encoded = append(encoded, base64.RawURLEncoding.EncodeToString([]byte(part)))
	}
	return strings.Join(encoded, ".")
}

func decodeResourceIDPart(value string) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", false
	}
	return string(raw), true
}
