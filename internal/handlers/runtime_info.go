package handlers

import "sync"

type RuntimeInfo struct {
	FrontendURL                 string
	FrontendDefaultPort         int
	FrontendPort                int
	FrontendDefaultPortOccupant string
	BackendURL                  string
	BackendDefaultPort          int
	BackendPort                 int
	BackendDefaultPortOccupant  string
	GatewayURL                  string
	RuntimeProtocol             int
	ConfigDir                   string
	DefaultConfigDir            string
	DatabasePath                string
	PendingConfigDir            string
}

var runtimeInfoState = struct {
	sync.RWMutex
	info RuntimeInfo
}{}

func SetRuntimeInfo(info RuntimeInfo) {
	runtimeInfoState.Lock()
	runtimeInfoState.info = info
	runtimeInfoState.Unlock()
}

func GetRuntimeInfo() RuntimeInfo {
	runtimeInfoState.RLock()
	defer runtimeInfoState.RUnlock()
	return runtimeInfoState.info
}
