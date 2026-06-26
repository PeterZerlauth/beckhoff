package ads

const (
	CmdReadDeviceInfo 	= 1
	CmdRead           	= 2
	CmdWrite          	= 3
	CmdReadState      	= 4
	CmdReadWrite      	= 9

	flagResponse 		= 0x0001
)

type ErrorCode uint32

const (
	NoError 			ErrorCode = 0x0000

	// General access / parameter errors
	AccessDenied     		ErrorCode = 0x0706
	InvalidParameter 		ErrorCode = 0x0705
	InvalidParamSize 		ErrorCode = 0x070C

	// Index / addressing errors
	InvalidIndexGroup  		ErrorCode = 0x0702
	InvalidIndexOffset 		ErrorCode = 0x0703

	// Symbol errors
	SymbolNotFound 			ErrorCode = 0x0710

	// Optional useful ones
	DeviceNotReady 			ErrorCode = 0x0707
	DeviceBusy     			ErrorCode = 0x0708
)

type ADSState uint16

const (
	STATE_INVALID      ADSState = 0
	STATE_IDLE         ADSState = 1
	STATE_RESET        ADSState = 2
	STATE_INIT         ADSState = 3
	STATE_START        ADSState = 4
	STATE_RUN          ADSState = 5
	STATE_STOP         ADSState = 6
	STATE_SAVECFG      ADSState = 7
	STATE_LOADCFG      ADSState = 8
	STATE_POWERFAILURE ADSState = 9
	STATE_POWERGOOD    ADSState = 10
	STATE_ERROR        ADSState = 11
	STATE_SHUTDOWN     ADSState = 12
	STATE_SUSPEND      ADSState = 13
	STATE_RESUME       ADSState = 14
	STATE_CONFIG       ADSState = 15 // system is in config mode
	STATE_RECONFIG     ADSState = 16 // system should restart in config mode
	STATE_MAXSTATES    ADSState = 17
)

// IndexGroups
const (
	IdxGetSymHandleByName        = 0x0000F003
	IdxReserved                  = 0x0000F004
	IdxReadWriteSymValueByHandle = 0x0000F005
	IdxReleaseSymHandle          = 0x0000F006
	IdxReadIWriteI               = 0x0000F020
	IdxReadIXWriteIX             = 0x0000F021
	IdxADSIGRP_IOIMAGE_RISIZE    = 0x0000F025
	IdxReadQWriteQ               = 0x0000F030
	IdxReadQXWriteQX             = 0x0000F031
	IdxADSIGRP_IOIMAGE_ROSIZE    = 0x0000F035
	IdxADSIGRP_SUMUP_READ        = 0x0000F080
	IdxADSIGRP_SUMUP_WRITE       = 0x0000F081
	IdxADSIGRP_SUMUP_READWRITE   = 0x0000F082
)
