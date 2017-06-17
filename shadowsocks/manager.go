package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

type Manager struct {
	// TODO should use lock for statistics update?
	Statistics        map[string]int
	pm                *PasswdManager
	controlClientAddr *net.UDPAddr
	serverAddr        *net.UDPAddr
	serverConn        *net.UDPConn
	isStopped         bool
}

func (mgr *Manager) UpdateStatistics(port string, dataLen int) {
	mgr.Statistics[port] += dataLen
}

func (mgr *Manager) ClearStatistics() {
	// just make a new one
	mgr.Statistics = make(map[string]int)
}

func (mgr *Manager) ManagementService() {
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8877")
	if err != nil {
		fmt.Errorf("fail to start management service: %v", err)
	}
	mgr.serverAddr = serverAddr
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		fmt.Errorf("fail to start management service: %v", err)
	}
	defer serverConn.Close()
	mgr.serverConn = serverConn

	log.Println("management service listening on 127.0.0.1:8877 (UDP)")

	for {
		if mgr.isStopped {
			break
		} else {
			buf := make([]byte, 1506)
			n, addr, err := serverConn.ReadFromUDP(buf)
			if err != nil {
				fmt.Println("fail to read from control client")
			}
			mgr.controlClientAddr = addr
			go handleManagementAPI(buf[0:n], mgr)
		}
	}
}

func (mgr *Manager) SendDataToControlClient(data []byte) error {
	if mgr.controlClientAddr != nil {
		_, err := mgr.serverConn.WriteToUDP(data, mgr.controlClientAddr)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (mgr *Manager) Run() {
	mgr.isStopped = false
	go mgr.ManagementService()
	go func(mgr *Manager) {
		// the loop below send statistics data to control client every 10 seconds
		for {
			if mgr.isStopped {
				break
			} else {
				stats, err := json.Marshal(mgr.Statistics)
				if err != nil {
					Debug.Println("fail to marshal statistics", err)
				}
				if mgr.controlClientAddr != nil {
					if err = mgr.SendDataToControlClient(stats); err != nil {
						fmt.Printf("fail to send statistics data", err)
					}
				}
				mgr.ClearStatistics()
				time.Sleep(10 * time.Second)
			}
		}
	}(mgr)
}

func (mgr *Manager) Stop() {
	mgr.isStopped = true
}

func (mgr *Manager) Destroy() {
	mgr.Stop()
}

func NewManager(pm *PasswdManager) *Manager {
	return &Manager{
		Statistics:        make(map[string]int),
		pm:                pm,
		controlClientAddr: nil,
		serverAddr:        nil,
		serverConn:        nil,
		isStopped:         true}
}

func parseData(data string) (string, string) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return data, ""
	}
	return parts[0], parts[1]
}

func handleManagementAPI(buf []byte, mgr *Manager) {
	data := string(buf)
	// fmt.Println("receive data: " + data)
	command, configStr := parseData(data)
	var err error
	switch command {
	case "add":
		// add server
		config := &Config{}
		if err := json.Unmarshal([]byte(configStr), config); err != nil {
			fmt.Println("error", err)
		}
		passwdManager.updatePortPasswd(strconv.Itoa(config.ServerPort), config.Password, config.Auth, mgr)
		err = mgr.SendDataToControlClient([]byte("ok"))
	case "remove":
		// remove server
		config := &Config{}
		if err := json.Unmarshal([]byte(configStr), config); err != nil {
			fmt.Println("error", err)
		}
		passwdManager.del(strconv.Itoa(config.ServerPort))
		err = mgr.SendDataToControlClient([]byte("ok"))
	case "ping":
		// send pong
		err = mgr.SendDataToControlClient([]byte("pong"))
	default:
		fmt.Printf("unknown command")
	}

	if err != nil {
		fmt.Println("error handle api", err)
	}
}
