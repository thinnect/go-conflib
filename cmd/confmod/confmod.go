// Author  Raido Pahtma
// License MIT

package main

import "fmt"
import "os"
import "log"
import "time"

import "encoding/binary"
import "strconv"
import "strings"

import "bytes"
import "errors"
import "bufio"

import "github.com/jessevdk/go-flags"
import "github.com/proactivity-lab/go-loggers"
import "github.com/proactivity-lab/go-sfconnection"
import "github.com/thinnect/go-conflib"

const ApplicationVersionMajor = 0
const ApplicationVersionMinor = 1
const ApplicationVersionPatch = 0

var ApplicationBuildDate string
var ApplicationBuildDistro string

func int32ToBytes(value int32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, value)
	return buf.Bytes()
}

func bytesToInt32(value []byte) (int32, error) {
	if len(value) == 4 {
		buf := bytes.NewBuffer(value)
		var v int32
		if err := binary.Read(buf, binary.BigEndian, &v); err == nil {
			return v, nil
		} else {
			return 0, err
		}
	}
	return 0, errors.New("bad length")
}

func loadConfList(infile string) (map[string]uint32, error) {
	in, err := os.Open(infile)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	scanner := bufio.NewScanner(bufio.NewReader(in))

	m := make(map[string]uint32)
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if len(t) > 0 && strings.HasPrefix(t, "#") == false {
			splits := strings.Split(t, ",")
			if len(splits) == 2 {
				name := strings.TrimSpace(splits[0])
				value := strings.TrimSpace(splits[1])
				if id, err := strconv.ParseUint(value, 16, 32); err == nil {
					m[name] = uint32(id)
				} else {
					return nil, err
				}
			} else {
				fmt.Printf("l %d %s\n", len(splits), splits)
			}
		}
	}
	return m, nil
}

func loadConfValues(infile string) (map[sfconnection.AMAddr]map[string]string, error) {
	in, err := os.Open(infile)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	scanner := bufio.NewScanner(bufio.NewReader(in))

	m := make(map[sfconnection.AMAddr]map[string]string)
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if len(t) > 0 && strings.HasPrefix(t, "#") == false {
			splits := strings.Split(t, ",")
			if len(splits) == 3 {
				s0 := strings.TrimSpace(splits[0])
				s1 := strings.TrimSpace(splits[1])
				s2 := strings.TrimSpace(splits[2])
				if saddr, err := strconv.ParseUint(s0, 16, 16); err == nil {
					addr := sfconnection.AMAddr(saddr)

					var nv map[string]string
					var ok bool
					if nv, ok = m[addr]; ok == false {
						nv = make(map[string]string)
						m[addr] = nv
					}
					nv[s1] = s2
				} else {
					return nil, err
				}
			} else {
				fmt.Printf("l %d %s\n", len(splits), splits)
			}
		}
	}
	return m, nil
}

type Options struct {
	Positional struct {
		ConnectionString string `description:"Connectionstring sf@HOST:PORT"`
		Conf             string `description:"Path to conf file"`
	} `positional-args:"yes"`

	ConfIdList string `short:"l" long:"idlist" default:"idlist.txt" description:"Name-ID mapping file"`

	Source sfconnection.AMAddr  `short:"s" long:"source" default:"0001" description:"Source of the packet (hex)"`
	Group  sfconnection.AMGroup `short:"g" long:"group" default:"22" description:"Packet AM Group (hex)"`

	Timeout uint `short:"t" long:"timeout" default:"5" description:"Timeout, seconds"`

	Get bool `long:"get" description:"Don't set, just get the values"`

	Debug       []bool `short:"D" long:"debug"   description:"Debug mode, print raw packets"`
	ShowVersion func() `short:"V" long:"version" description:"Show application version"`
}

func mainfunction() int {

	var opts Options
	opts.ShowVersion = func() {
		if ApplicationBuildDate == "" {
			ApplicationBuildDate = "YYYY-mm-dd_HH:MM:SS"
		}
		if ApplicationBuildDistro == "" {
			ApplicationBuildDistro = "unknown"
		}
		fmt.Printf("deviceparameter %d.%d.%d (%s %s)\n", ApplicationVersionMajor, ApplicationVersionMinor, ApplicationVersionPatch, ApplicationBuildDate, ApplicationBuildDistro)
		os.Exit(0)
	}

	_, err := flags.Parse(&opts)
	if err != nil {
		fmt.Printf("Argument parser error: %s\n", err)
		return 1
	}

	host, port, err := sfconnection.ParseSfConnectionString(opts.Positional.ConnectionString)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		return 1
	}

	sfc := sfconnection.NewSfConnection()
	cpm := conflib.NewConfParameterManager(sfc, opts.Source, opts.Group)
	cpm.SetRetries(0)
	cpm.SetTimeout(time.Duration(opts.Timeout) * time.Second)
	defer cpm.Close()

	logger := logsetup(len(opts.Debug))
	if len(opts.Debug) > 0 {
		sfc.SetLoggers(logger)
	}
	cpm.SetLoggers(logger)

	// Parse idlist
	if conflist, err := loadConfList(opts.ConfIdList); err == nil {
		logger.Debug.Printf("Conflist has %d mappings\n", len(conflist))
		for k, v := range conflist {
			cpm.AddMapping(k, v)
		}
	} else {
		logger.Error.Printf("Conflist parsing failed %s\n", err)
		return 1
	}

	// Parse parameters
	if paramlist, err := loadConfValues(opts.Positional.Conf); err == nil {
		logger.Debug.Printf("List has %d targets\n", len(paramlist))
		if len(paramlist) > 0 {

			// Connect to the host
			err = sfc.Connect(host, port)
			if err != nil {
				logger.Error.Printf("Unable to connect to %s:%d\n", host, port)
				return 1
			}
			logger.Info.Printf("Connected to %s:%d\n", host, port)
			defer sfc.Disconnect()

			// Configure targets
			for len(paramlist) > 0 {
				for target, params := range paramlist {
					logger.Debug.Printf("Target %v has %d parameters\n", target, len(params))
					logger.Info.Printf("Configuring %v\n", target)

					for name, value := range params {
						nextnode := false

						if pvalue, err := strconv.ParseInt(value, 10, 32); err == nil {
							ivalue := int32(pvalue)

							next := false
							for next != true {

								if current, err := cpm.GetValue(target, name); err == nil {
									cvalue, _ := bytesToInt32(current.Value)
									if opts.Get == false && cvalue != ivalue {
										logger.Info.Printf("  %-10s: %d -> %d\n", name, cvalue, ivalue)
										err := cpm.SetValue(target, name, int32ToBytes(ivalue))
										if err == nil {
											next = true
											delete(params, name)
										} else {
											logger.Info.Printf("  %-10s error: %v\n", name, err)
										}
									} else {
										logger.Info.Printf("  %-10s: %d\n", name, cvalue)
										next = true
										delete(params, name)
									}
								} else {
									logger.Info.Printf("  %-10s error: %v\n", name, err)
								}

								if next == false {
									logger.Info.Printf("  (l)ater/(s)kip/(n)next/(d)rop/(a)bort? [retry]")
									var input string
									fmt.Scanln(&input)
									if input == "a" || input == "A" {
										return 1
									} else if input == "l" || input == "L" {
										next = true
									} else if input == "s" || input == "S" {
										delete(params, name)
										next = true
									} else if input == "n" || input == "N" {
										nextnode = true
										next = true
									} else if input == "d" || input == "D" {
										delete(paramlist, target)
										nextnode = true
										next = true
									}
								}

							}

						} else {
							logger.Error.Printf("  %-10s value %s cannot be used!\n", name, value)
						}

						if nextnode {
							break
						}
					}

					if len(params) == 0 {
						delete(paramlist, target)
					}

				}
			}
		}
	} else {
		logger.Error.Printf("List parsing failed %v\n", err)
		return 1
	}

	time.Sleep(100 * time.Millisecond)
	return 0
}

func logsetup(debuglevel int) *loggers.DIWEloggers {
	logger := loggers.New()
	logformat := log.Ldate | log.Ltime | log.Lmicroseconds

	if debuglevel > 1 {
		logformat = logformat | log.Lshortfile
	}

	if debuglevel > 0 {
		logger.SetDebugLogger(log.New(os.Stdout, "DEBUG: ", logformat))
		logger.SetInfoLogger(log.New(os.Stdout, "INFO:  ", logformat))
	} else {
		logger.SetInfoLogger(log.New(os.Stdout, "", logformat))
	}
	logger.SetWarningLogger(log.New(os.Stdout, "WARN:  ", logformat))
	logger.SetErrorLogger(log.New(os.Stdout, "ERROR: ", logformat))
	return logger
}

func main() {
	os.Exit(mainfunction())
}
