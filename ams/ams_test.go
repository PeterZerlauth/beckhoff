package ams

import (
    "reflect"
    "testing"
)

func TestParseNetId_Valid(t *testing.T) {
    tests := []struct {
        input    string
        expected NetId
    }{
        {"1.2.3.4.5.6", NetId{1, 2, 3, 4, 5, 6}},
        {"0.0.0.0.0.0", NetId{0, 0, 0, 0, 0, 0}},
        {"255.255.255.255.255.255", NetId{255, 255, 255, 255, 255, 255}},
    }

    for _, test := range tests {
        t.Run(test.input, func(t *testing.T) {
            result, err := ParseNetId(test.input)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !reflect.DeepEqual(result, test.expected) {
                t.Errorf("expected %v, got %v", test.expected, result)
            }
        })
    }
}

func TestParseNetId_Invalid(t *testing.T) {
    tests := []string{
        "",                       // empty
        "1.2.3.4.5",              // too short
        "1.2.3.4.5.6.7",          // too long
        "1.2.3.4.5.a",            // non-numeric
        "1.2.3.4.5.-1",           // negative
        "1.2.3.4.5.256",          // out of range
    }

    for _, input := range tests {
        t.Run(input, func(t *testing.T) {
            _, err := ParseNetId(input)
            if err == nil {
                t.Errorf("expected error for input %q, got nil", input)
            }
        })
    }
}

func TestNetId_String(t *testing.T) {
    tests := []struct {
        input    NetId
        expected string
    }{
        {NetId{1, 2, 3, 4, 5, 6}, "1.2.3.4.5.6"},
        {NetId{0, 0, 0, 0, 0, 0}, "0.0.0.0.0.0"},
        {NetId{255, 255, 255, 255, 255, 255}, "255.255.255.255.255.255"},
    }

    for _, test := range tests {
        t.Run(test.expected, func(t *testing.T) {
            result := test.input.ToString() // ✅ fixed
            // or: result := test.input.String() if you named it String()

            if result != test.expected {
                t.Errorf("expected %s, got %s", test.expected, result)
            }
        })
    }
}
