package server

import (
	"sync"

	"github.com/PeterZerlauth/beckhoff/ads"
)

// ADSDataType mirrors AdsDataType from the C++ SDK
type ADSDataType uint32

const (
	TypeVoid    ADSDataType = 0
	TypeBool    ADSDataType = 33
	TypeByte    ADSDataType = 17
	TypeWord    ADSDataType = 18
	TypeDWord   ADSDataType = 19
	TypeInt8    ADSDataType = 16
	TypeInt     ADSDataType = 2
	TypeDInt    ADSDataType = 3
	TypeReal    ADSDataType = 4
	TypeLReal   ADSDataType = 5
	TypeString  ADSDataType = 30
	TypeTime    ADSDataType = 11
	TypeUserDef ADSDataType = 65
)

// SymbolFlags mirrors the flags field of AdsSymbolEntry
type SymbolFlags uint32

const (
	FlagPersistent    SymbolFlags = 1 << 0
	FlagReadOnly      SymbolFlags = 1 << 1
	FlagRetain        SymbolFlags = 1 << 2
	FlagBitValue      SymbolFlags = 1 << 5 // size == 0 means bit
	FlagExtendedFlags SymbolFlags = 1 << 11
)

// IndexKey represents the (iGroup, iOffs) addressing pair from AdsSymbolEntry.
// This is the canonical ADS address — handle is a convenience alias.
type IndexKey struct {
	Group  uint32
	Offset uint32
}

// Symbol now carries all fields from AdsSymbolEntry, not just name + bytes.
type Symbol struct {
	Handle   uint32
	Index    IndexKey
	Name     string
	TypeName string // e.g. "BOOL", "INT", "MyStruct"
	Comment  string
	DataType ADSDataType
	Flags    SymbolFlags
	Size     uint32 // byte size; 0 means bit-sized (FlagBitValue)
	Data     []byte
}

func (s *Symbol) IsReadOnly() bool   { return s.Flags&FlagReadOnly != 0 }
func (s *Symbol) IsBitValue() bool   { return s.Flags&FlagBitValue != 0 || s.Size == 0 }
func (s *Symbol) IsPersistent() bool { return s.Flags&FlagPersistent != 0 }

// SymbolTable adds a third index on (IndexGroup, IndexOffset), matching
// how real ADS devices are addressed.
type SymbolTable struct {
	mu          sync.RWMutex
	nextHandle  uint32
	symbols     []*Symbol
	nameIndex   map[string]*Symbol
	handleIndex map[uint32]*Symbol
	groupIndex  map[IndexKey]*Symbol // new: ADS-native addressing
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		nextHandle:  1,
		symbols:     make([]*Symbol, 0),
		nameIndex:   make(map[string]*Symbol),
		handleIndex: make(map[uint32]*Symbol),
		groupIndex:  make(map[IndexKey]*Symbol),
	}
}

// AddFull inserts a fully-described symbol, replacing any existing entry
// with the same name.
func (s *SymbolTable) AddFull(sym Symbol) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.nameIndex[sym.Name]; ok {
		// Update in-place: remove stale group key if index changed
		if existing.Index != sym.Index {
			delete(s.groupIndex, existing.Index)
		}
		sym.Handle = existing.Handle
		*existing = sym
		existing.Handle = sym.Handle
		s.groupIndex[sym.Index] = existing
		s.handleIndex[sym.Handle] = existing
		return sym.Handle
	}

	sym.Handle = s.nextHandle
	s.nextHandle++

	entry := &sym
	s.symbols = append(s.symbols, entry)
	s.nameIndex[entry.Name] = entry
	s.handleIndex[entry.Handle] = entry
	s.groupIndex[entry.Index] = entry
	return entry.Handle
}

// Add is the original convenience method, now backed by AddFull.
func (s *SymbolTable) Add(name string, data []byte) uint32 {
	return s.AddFull(Symbol{
		Name: name,
		Data: append([]byte{}, data...),
	})
}

// GetHandle returns the handle for a named symbol.
func (s *SymbolTable) GetHandle(name string) (uint32, ads.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sym, ok := s.nameIndex[name]
	if !ok {
		return 0, ads.SymbolNotFound
	}
	return sym.Handle, ads.NoError
}

// GetByIndex looks up a symbol by its ADS index address (group + offset).
func (s *SymbolTable) GetByIndex(group, offset uint32) (*Symbol, ads.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sym, ok := s.groupIndex[IndexKey{group, offset}]
	if !ok {
		return nil, ads.SymbolNotFound
	}
	return sym, ads.NoError
}

// GetMeta returns a copy of the symbol's metadata without its data payload.
func (s *SymbolTable) GetMeta(h uint32) (Symbol, ads.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sym, ok := s.handleIndex[h]
	if !ok {
		return Symbol{}, ads.SymbolNotFound
	}
	clone := *sym
	clone.Data = nil
	return clone, ads.NoError
}

// Read returns a copy of the symbol's data.
func (s *SymbolTable) Read(h uint32) ([]byte, ads.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sym, ok := s.handleIndex[h]
	if !ok {
		return nil, ads.SymbolNotFound
	}
	return append([]byte{}, sym.Data...), ads.NoError
}

// Write validates the write against size and read-only flag before storing.
func (s *SymbolTable) Write(h uint32, data []byte) ads.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	sym, ok := s.handleIndex[h]
	if !ok {
		return ads.SymbolNotFound
	}
	if sym.IsReadOnly() {
		return ads.AccessDenied
	}
	// Only enforce size when it was explicitly set (non-zero, non-bit)
	if sym.Size > 0 && !sym.IsBitValue() && uint32(len(data)) != sym.Size {
		return ads.InvalidParamSize
	}
	sym.Data = append([]byte{}, data...)
	return ads.NoError
}

// Release removes a symbol from all three indexes.
func (s *SymbolTable) Release(h uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sym, ok := s.handleIndex[h]
	if !ok {
		return
	}
	delete(s.nameIndex, sym.Name)
	delete(s.handleIndex, h)
	delete(s.groupIndex, sym.Index)
	for i, v := range s.symbols {
		if v.Handle == h {
			last := len(s.symbols) - 1
			s.symbols[i] = s.symbols[last]
			s.symbols = s.symbols[:last]
			break
		}
	}
}

// Snapshot returns metadata for all symbols (no data payloads) — useful
// for implementing the ADS ReadSymbolTable command.
func (s *SymbolTable) Snapshot() []Symbol {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Symbol, len(s.symbols))
	for i, sym := range s.symbols {
		out[i] = *sym
		out[i].Data = nil
	}
	return out
}

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
