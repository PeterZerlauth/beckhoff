package server

import (
    "sync"

    "beckhoff/ads"
)

type Symbol struct {
    Handle uint32
    Name   string
    Data   []byte
}

type SymbolTable struct {
    mu sync.RWMutex

    nextHandle uint32
    symbols    []*Symbol

    nameIndex   map[string]*Symbol
    handleIndex map[uint32]*Symbol
}

func NewSymbolTable() *SymbolTable {
    return &SymbolTable{
        nextHandle:  1,
        symbols:     make([]*Symbol, 0),
        nameIndex:   make(map[string]*Symbol),
        handleIndex: make(map[uint32]*Symbol),
    }
}

// GetHandle returns an existing handle or creates a new Symbol
func (s *SymbolTable) GetHandle(name string) (uint32, ads.ErrorCode) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    if sym, ok := s.nameIndex[name]; ok {
        return sym.Handle, ads.NoError
    }

    return 0, ads.SymbolNotFound
}

// Add inserts or updates a symbol
func (s *SymbolTable) Add(name string, data []byte) uint32 {
    s.mu.Lock()
    defer s.mu.Unlock()

    if sym, ok := s.nameIndex[name]; ok {
        sym.Data = append([]byte{}, data...)
        return sym.Handle
    }

    sym := &Symbol{
        Handle: s.nextHandle,
        Name:   name,
        Data:   append([]byte{}, data...),
    }

    s.nextHandle++

    s.symbols = append(s.symbols, sym)
    s.nameIndex[name] = sym
    s.handleIndex[sym.Handle] = sym

    return sym.Handle
}

// Read symbol data
func (s *SymbolTable) Read(h uint32) ([]byte, ads.ErrorCode) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    sym, ok := s.handleIndex[h]
    if !ok {
        return nil, ads.SymbolNotFound
    }

    return append([]byte{}, sym.Data...), ads.NoError
}

// Write symbol data
func (s *SymbolTable) Write(h uint32, data []byte) ads.ErrorCode {
    s.mu.Lock()
    defer s.mu.Unlock()

    sym, ok := s.handleIndex[h]
    if !ok {
        return ads.SymbolNotFound
    }

    sym.Data = append([]byte{}, data...)
    return ads.NoError
}

// Release removes a symbol completely
func (s *SymbolTable) Release(h uint32) {
    s.mu.Lock()
    defer s.mu.Unlock()

    sym, ok := s.handleIndex[h]
    if !ok {
        return
    }

    delete(s.nameIndex, sym.Name)
    delete(s.handleIndex, h)

    // O(1) delete (swap with last)
    for i, v := range s.symbols {
        if v.Handle == h {
            last := len(s.symbols) - 1
            s.symbols[i] = s.symbols[last]
            s.symbols = s.symbols[:last]
            break
        }
    }
}

// Optional helpers

func (s *SymbolTable) Exists(name string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    _, ok := s.nameIndex[name]
    return ok
}

func (s *SymbolTable) Name(h uint32) (string, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    sym, ok := s.handleIndex[h]
    if !ok {
        return "", false
    }
    return sym.Name, true
}

func (s *SymbolTable) Len() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.symbols)
}