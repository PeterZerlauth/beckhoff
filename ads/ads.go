package ads

const (
    TcpHeaderSize = 6

    CmdReadDeviceInfo = 1
    CmdRead           = 2
    CmdWrite          = 3
    CmdReadState      = 4
    CmdReadWrite      = 9


    flagResponse        = 0x0001
    workers             = 8
)

type ErrorCode uint32
const (
    NoError         ErrorCode = 0x0
    SymbolNotFound  ErrorCode = 0x710
)