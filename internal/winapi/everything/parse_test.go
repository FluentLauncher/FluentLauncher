//go:build windows

package everything

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestParseResults(t *testing.T) {
	name := append(utf16.Encode([]rune("a.txt")), 0)
	dir := append(utf16.Encode([]rune(`C:\Temp`)), 0)
	header := 28 + 12
	payload := make([]byte, header+len(name)*2+len(dir)*2)
	binary.LittleEndian.PutUint32(payload[20:], 1)
	binary.LittleEndian.PutUint32(payload[28:], 0)
	binary.LittleEndian.PutUint32(payload[32:], uint32(header))
	binary.LittleEndian.PutUint32(payload[36:], uint32(header+len(name)*2))
	for i, value := range name {
		binary.LittleEndian.PutUint16(payload[header+i*2:], value)
	}
	for i, value := range dir {
		binary.LittleEndian.PutUint16(payload[header+len(name)*2+i*2:], value)
	}
	results, err := parseResults(payload, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Entry.Title != "a.txt" || results[0].Entry.LaunchTarget != `C:\Temp\a.txt` {
		t.Fatalf("results = %+v", results)
	}
}
