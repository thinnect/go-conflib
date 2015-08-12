// Author  Raido Pahtma
// License MIT

package conflib

const CP_REQ_CONF_PARAM_BY_ID = 0x04
const CP_CONF_PARAM = 0x05
const CP_SET_CONF_PARAM = 0x06
const CP_ACK = 0x0A

// Set message
// CP_SET_CONF_PARAM
type MessageSetConf struct {
	Header uint8
	Id     uint32
	Value  uint32
}

// Ack message
// CP_ACK
type MessageAckConf struct {
	Header    uint8
	Result    uint8
	Seq       uint8
	Id        uint32
	ErrorCode uint8
}

// Get message
// CP_REQ_CONF_PARAM_BY_ID
type MessageGetConf struct {
	Header uint8
	Id     uint32
}

// Info
// CP_CONF_PARAM
type MessageConfInfo struct {
	Header       uint8
	Id           uint32
	Seq          uint8
	DataType     uint8
	DefaultValue uint32
	MinValue     uint32
	MaxValue     uint32
	Value        uint32
	StorageType  uint8
}
