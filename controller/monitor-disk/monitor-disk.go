package monitor_disk

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	glog "k8s.io/klog"

	v1 "kubevirt.io/hostpath-provisioner/controller/monitor-disk/api/v1"
)

type ModifyDiskArgs struct {
	// NodeName as diskMonitorCR name
	CRName          string
	Namespace       string
	Path            string
	Operation       string
	OwnerReferences string
	DiskInfo        *v1.DiskDetail
	Require         *resource.Quantity
}

var gvr = schema.GroupVersionResource{
	Group:    "diskmonitor.domain",
	Version:  "v1",
	Resource: "diskmonitors",
}

var gvk = schema.GroupVersionKind{
	Group:   "diskmonitor.domain",
	Version: "v1",
	Kind:    "DiskMonitor",
}

const (
	OPERATE_UPDATE = "update"
	OPERATE_DELETE = "delete"
)

func List(namespace string) (*v1.DiskMonitorList, error) {
	client := getDynamicClientSet()
	list, err := client.Resource(gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	data, err := list.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var diskMonitorList v1.DiskMonitorList
	if err := json.Unmarshal(data, &diskMonitorList); err != nil {
		return nil, err
	}
	js, _ := json.Marshal(diskMonitorList.Items)
	glog.Info("DiskMonitorList list", string(js))
	return &diskMonitorList, nil
}

func Get(namespace string, name string) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	utd, err := client.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		glog.Error("get namespace/name %v/%v DiskMonitor err: %v", namespace, name, err)
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		glog.Error("get DiskMonitor Marshal err:", err)
		return nil, err
	}
	var diskMonitor v1.DiskMonitor
	if err := json.Unmarshal(data, &diskMonitor); err != nil {
		glog.Error("get DiskMonitor UnMarshal err:", err)
		return nil, err
	}
	return &diskMonitor, nil
}

func Delete(namespace string, name string) error {
	client := getDynamicClientSet()
	return client.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func Create(ns string, monitor *v1.DiskMonitor) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	js, _ := json.Marshal(monitor)
	glog.Info("monitor create info: ", string(js))
	obj, err := Convert2Unstruct(monitor)
	if err != nil {
		return nil, err
	}
	utd, err := client.Resource(gvr).Namespace(ns).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		glog.Error("DiskMonitor create err ", err)
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		glog.Error("DiskMonitor create MarshalJSON err", err)
		return nil, err
	}
	var diskMonitor v1.DiskMonitor
	if err := json.Unmarshal(data, &diskMonitor); err != nil {
		glog.Error("DiskMonitor create Unmarshal err", err)
		return nil, err
	}
	return &diskMonitor, nil
}

func Update(ns string, monitor *v1.DiskMonitor) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	obj, err := Convert2Unstruct(monitor)
	if err != nil {
		return nil, err
	}
	utd, err := client.Resource(gvr).Namespace(ns).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		glog.Error(err)
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		glog.Error("update DiskMonitor err: ", err)
		return nil, err
	}
	var diskMonitor v1.DiskMonitor
	if err := json.Unmarshal(data, &diskMonitor); err != nil {
		glog.Error("update DiskMonitor Unmarshal err", err)
		return nil, err
	}
	return &diskMonitor, nil
}

func Convert2Unstruct(diskMonitor *v1.DiskMonitor) (*unstructured.Unstructured, error) {
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	bt, _ := json.Marshal(diskMonitor)
	if _, _, err := decoder.Decode(bt, &gvk, obj); err != nil {
		glog.Error("Convert2Unstruct", err)
		return nil, err
	}
	return obj, nil
}

func getDynamicClientSet() dynamic.Interface {
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatal("Failed to create config: %v", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return client
}
