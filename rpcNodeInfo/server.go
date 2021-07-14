package rpcNodeInfo

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"runtime"

	glog "k8s.io/klog"
)

type NodeNICInfo struct {
	InterfaceName, IP, Mac string
}
type NodeNICsInfo struct {
	HostName string
	CoreNum  int
	NICs     []NodeNICInfo
}

type NodeInfo int

func (*NodeInfo) GetNICInfo(args *string, nodeNICInfos *NodeNICsInfo) error {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("fail to get net interfaces: %v", err)
		return err
	}
	for _, netInterface := range netInterfaces {
		macAddr := netInterface.HardwareAddr.String()
		if len(macAddr) == 0 {
			continue
		}
		addrs, _ := netInterface.Addrs()
		for _, addr := range addrs {
			nodeNICInfos.NICs = append(nodeNICInfos.NICs, NodeNICInfo{
				InterfaceName: netInterface.Name,
				IP:            addr.String(),
				Mac:           netInterface.HardwareAddr.String(),
			})
		}
	}
	nodeNICInfos.CoreNum = runtime.NumCPU()
	glog.Info("node NIC info ", nodeNICInfos)
	return nil
}

func Run() {
	nodeInfo := new(NodeInfo)
	if err := rpc.Register(nodeInfo); err != nil {
		glog.Error("register rpc err: ", err)
		return
	}
	rpc.HandleHTTP()
	if err := http.ListenAndServe(":50234", nil); err != nil {
		log.Fatal("serve error:", err)
	}
}
