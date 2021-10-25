package rpcNodeInfo

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/mem"
	glog "k8s.io/klog"
	"kubevirt.io/hostpath-provisioner/tests"
)

type NodeNICInfo struct {
	InterfaceName, IP, Mac string
}
type NodeNICsInfo struct {
	HostName string
	CoreNum  int
	Mem      uint64
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
			var ip []string
			if strings.Contains(addr.String(), ":") {
				continue
			} else {
				ip = strings.Split(addr.String(), "/")
			}
			memory, _ := mem.VirtualMemory()
			nodeNICInfos.Mem = memory.Total / uint64(tests.GiB)
			nodeNICInfos.NICs = append(nodeNICInfos.NICs, NodeNICInfo{
				InterfaceName: netInterface.Name,
				IP:            ip[0],
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
	NodeInfoPort := os.Getenv("NODE_INFO_PORT")
	if len(NodeInfoPort) == 0 {
		NodeInfoPort = "50234"
	}
	if err := http.ListenAndServe(":"+NodeInfoPort, nil); err != nil {
		log.Fatal("serve error:", err)
	}
}
