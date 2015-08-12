// Author  Raido Pahtma
// License MIT

package conflib

import "fmt"
import "time"
import "errors"
import "bytes"
import "encoding/binary"

import "github.com/proactivity-lab/go-loggers"
import "github.com/proactivity-lab/go-sfconnection"

type ConfParameter struct {
	Name   string
	Id     uint32 // Will not be used in next protocol version
	Type   uint8
	Seqnum uint8
	Value  []byte
	Error  error
}

const (
	CP_TYPE_RAW    = 0x00
	CP_TYPE_UINT8  = 0x01
	CP_TYPE_UINT16 = 0x02
	CP_TYPE_UINT32 = 0x04
	CP_TYPE_UINT64 = 0x08

	CP_TYPE_STRING = 0x80
	CP_TYPE_INT8   = 0x81
	CP_TYPE_INT16  = 0x82
	CP_TYPE_INT32  = 0x84
	CP_TYPE_INT64  = 0x88
)

const AMID_CONF = 0x90

type ConfParameterManager struct {
	loggers.DIWEloggers
	sfc *sfconnection.SfConnection
	dsp *sfconnection.MessageDispatcher

	timeout time.Duration
	retries int

	// Map will become reduntant in next version of protocol
	mapnameid map[string]uint32

	receive chan *sfconnection.Message

	done   chan bool
	closed bool
}

type ParameterError struct{ s string }
type TimeoutError struct{ s string }

func (self ParameterError) Error() string { return self.s }
func NewParameterError(text string) error { return &ParameterError{text} }
func (self TimeoutError) Error() string   { return self.s }
func NewTimeoutError(text string) error   { return &TimeoutError{text} }

func NewConfParameterManager(sfc *sfconnection.SfConnection, source sfconnection.AMAddr, group sfconnection.AMGroup) *ConfParameterManager {
	dpm := new(ConfParameterManager)
	dpm.InitLoggers()
	dpm.done = make(chan bool)
	dpm.closed = false
	dpm.receive = make(chan *sfconnection.Message)
	dpm.timeout = time.Second
	dpm.retries = 0

	dpm.dsp = sfconnection.NewMessageDispatcher(sfconnection.NewMessage(group, source))
	dpm.dsp.RegisterMessageReceiver(AMID_CONF, dpm.receive)

	dpm.mapnameid = make(map[string]uint32)

	dpm.sfc = sfc
	dpm.sfc.AddDispatcher(dpm.dsp)

	go dpm.run()
	return dpm
}

func (self *ConfParameterManager) AddMapping(name string, id uint32) {
	self.mapnameid[name] = id
}

func (self *ConfParameterManager) resolveId(name string) (uint32, error) {
	if val, ok := self.mapnameid[name]; ok {
		return val, nil
	}
	return 0, NewParameterError("ID not known for parameter")
}

func (self *ConfParameterManager) resolveValue(value []byte) (uint32, error) {
	if len(value) == 4 {
		buf := bytes.NewBuffer(value)
		var v uint32
		if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
			return v, nil
		} else {
			return 0, err
		}
	}
	return 0, NewParameterError("bad length")
}

func (self *ConfParameterManager) breakValue(value uint32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, value)
	return buf.Bytes(), err
}

func (self *ConfParameterManager) SetTimeout(timeout time.Duration) {
	self.timeout = timeout
}

func (self *ConfParameterManager) SetRetries(retries int) {
	self.retries = retries
}

func (self *ConfParameterManager) GetValue(addr sfconnection.AMAddr, name string) (*ConfParameter, error) {
	// Interrupt the run goroutine
	self.done <- true

	var result error = errors.New("disabled")

	for retries := 0; retries <= self.retries; retries++ {
		// Send get request
		msg := self.dsp.NewMessage()
		msg.SetType(AMID_CONF)
		msg.SetDestination(addr)
		payload := new(MessageGetConf)
		payload.Header = CP_REQ_CONF_PARAM_BY_ID
		payload.Id, _ = self.resolveId(name)
		msg.Payload = sfconnection.SerializePacket(payload)
		self.sfc.Send(msg)

		// Wait for value
		dp, err := self.waitValueId(name)
		if err == nil {
			go self.run()
			return dp, nil
		} else {
			result = err
			if _, ok := err.(*ParameterError); ok {
				break
			}
		}
	}

	go self.run()
	return nil, result
}

func (self *ConfParameterManager) SetValue(addr sfconnection.AMAddr, name string, value []byte) error {
	// Interrupt the run goroutine
	self.done <- true

	var result error = errors.New("disabled")

	for retries := 0; retries <= self.retries; retries++ {
		// Send set request
		msg := self.dsp.NewMessage()
		msg.SetType(AMID_CONF)
		msg.SetDestination(addr)
		payload := new(MessageSetConf)
		payload.Header = CP_SET_CONF_PARAM
		payload.Id, _ = self.resolveId(name)
		payload.Value, _ = self.resolveValue(value)
		msg.Payload = sfconnection.SerializePacket(payload)
		self.sfc.Send(msg)

		// Wait for value
		result = self.waitAckId(name)
		if result == nil {
			break
		}
	}

	go self.run()
	return result
}

func (self *ConfParameterManager) receivedPacket(msg *sfconnection.Message) {
	self.Debug.Printf("%s\n", msg)
}

func (self *ConfParameterManager) waitValueId(name string) (*ConfParameter, error) {
	start := time.Now()
	id, _ := self.resolveId(name)
	for {
		select {
		case msg := <-self.receive:
			if len(msg.Payload) > 0 {
				if msg.Payload[0] == CP_CONF_PARAM {
					p := new(MessageConfInfo)
					if err := sfconnection.DeserializePacket(p, msg.Payload); err == nil {
						if p.Id == id {
							v, _ := self.breakValue(p.Value)
							return &ConfParameter{name, p.Id, p.DataType, p.Seq, v, nil}, nil
						}
					} else {
						self.Error.Printf("Deserialize error %s %s\n", err, msg)
					}
				} else {
					self.receivedPacket(msg)
				}
			}
		case <-time.After(remaining(start, self.timeout)):
			return nil, NewTimeoutError(fmt.Sprintf("Timeout for parameter \"%s\"!", name))
		}
	}
}

func (self *ConfParameterManager) waitAckId(name string) error {
	start := time.Now()
	id, _ := self.resolveId(name)
	for {
		select {
		case msg := <-self.receive:
			if len(msg.Payload) > 0 {
				if msg.Payload[0] == CP_ACK {
					p := new(MessageAckConf)
					if err := sfconnection.DeserializePacket(p, msg.Payload); err == nil {
						if p.Id == id {
							if p.Result == 0 {
								return nil
							}
							return errors.New(fmt.Sprintf("Something went wrong with parameter \"%s\", error %d!", name, p.ErrorCode))
						}
					} else {
						self.Error.Printf("Deserialize error %s %s\n", err, msg)
					}
				} else {
					self.receivedPacket(msg)
				}
			}
		case <-time.After(remaining(start, self.timeout)):
			return NewTimeoutError(fmt.Sprintf("Timeout for parameter \"%s\"!", name))
		}
	}
}

func (self *ConfParameterManager) run() {
	self.Debug.Printf("CPM running\n")
	for {
		select {
		case msg := <-self.receive:
			self.receivedPacket(msg)
		case done := <-self.done:
			if done {
				self.Debug.Printf("CPM interrupted\n")
			} else {
				self.Debug.Printf("CPM closed\n")
			}
			return
		}
	}
}

func (self *ConfParameterManager) Close() error {
	if !self.closed {
		self.closed = true
		close(self.done)
		return nil
	}
	return errors.New("Close has already been called!")
}

func (self *ConfParameter) String() string {
	if self.Type == CP_TYPE_RAW {
		return fmt.Sprintf("%X", self.Value)
	} else if self.Type == CP_TYPE_STRING {
		return string(self.Value)
	} else {
		s := fmt.Sprintf("%v", self.Value)
		buf := bytes.NewBuffer(self.Value)
		if self.Type == CP_TYPE_UINT8 {
			var v uint8
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_UINT16 {
			var v uint16
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_UINT32 {
			var v uint32
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_UINT64 {
			var v uint64
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_INT8 {
			var v int8
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_INT16 {
			var v int16
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_INT32 {
			var v int32
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		} else if self.Type == CP_TYPE_INT64 {
			var v int64
			if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
				s = fmt.Sprintf("%d", v)
			}
		}
		return s
	}
}

func remaining(start time.Time, timeout time.Duration) time.Duration {
	elapsed := time.Since(start)
	if elapsed < timeout {
		return timeout - elapsed
	}
	return 0
}
