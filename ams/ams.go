package ams

import (
    "fmt"
    "strconv"
    "strings"
)
const (
    HeaderSize = 32
)

type NetId [6]byte

type Address struct {
    NetId NetId
    Port  uint16
}

// ✅ String method (idiomatic Go)
func (n NetId) String() string {
    return fmt.Sprintf("%d.%d.%d.%d.%d.%d",
        n[0], n[1], n[2], n[3], n[4], n[5])
}

// ✅ Parse string → NetId
func ParseNetId(s string) (NetId, error) {
    var n NetId

    parts := strings.Split(s, ".")
    if len(parts) != 6 {
        return n, fmt.Errorf("invalid NetId format")
    }

    for i := 0; i < 6; i++ {
        v, err := strconv.Atoi(parts[i])
        if err != nil || v < 0 || v > 255 {
            return n, fmt.Errorf("invalid value: %s", parts[i])
        }
        n[i] = byte(v)
    }

    return n, nil
}