// Author  Raido Pahtma
// License MIT

package conflib

import "testing"

import "log"
import "os"
import "fmt"
import "time"
import "github.com/proactivity-lab/go-sfconnection"

import "encoding/binary"
import "bytes"

func breakValue(value uint32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, value)
	return buf.Bytes()
}

func TestConflib(t *testing.T) {
	sfc := sfconnection.NewSfConnection()
	dp := NewConfParameterManager(sfc, 0x22, 0x0001)

	logformat := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	debuglogger := log.New(os.Stdout, "DEBUG: ", logformat)
	infologger := log.New(os.Stdout, "INFO:  ", logformat)
	warninglogger := log.New(os.Stdout, "WARN:  ", logformat)
	errorlogger := log.New(os.Stdout, "ERROR: ", logformat)

	sfc.SetDebugLogger(debuglogger)
	sfc.SetInfoLogger(infologger)
	sfc.SetWarningLogger(warninglogger)
	sfc.SetErrorLogger(errorlogger)

	dp.SetDebugLogger(debuglogger)
	dp.SetInfoLogger(infologger)
	dp.SetWarningLogger(warninglogger)
	dp.SetErrorLogger(errorlogger)

	sfc.Autoconnect("propi027.local", 9002, 30*time.Second)

	time.Sleep(time.Second)

	dp.AddMapping("parameter", 1)
	dp.AddMapping("dummy", 0xFF)

	v1, err := dp.GetValue(0xEEA2, "parameter")
	fmt.Printf("%v %v\n", v1, err)

	err = dp.SetValue(0xEEA2, "parameter", breakValue(uint32(time.Now().Unix())))
	fmt.Printf("s %v\n", err)

	v2, err := dp.GetValue(0xEEA2, "parameter")
	fmt.Printf("%v %v\n", v2, err)

	dp.SetTimeout(0)
	v3, err := dp.GetValue(0xEEA2, "dummy")
	fmt.Printf("%v %v\n", v3, err)

	dp.SetTimeout(time.Second)
	v4, err := dp.GetValue(0xEEA2, "dummy")
	fmt.Printf("%v %v\n", v4, err)

	err = dp.SetValue(0xEEA2, "dummy", breakValue(0))
	fmt.Printf("s %v\n", err)

	dp.Close()
	sfc.Disconnect()
}
