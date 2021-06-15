package monitor_disk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/common/log"
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
	CRName    string
	Namespace string
	Path      string
	Operation string
	DiskInfo  *v1.DiskDetail
	Require   *resource.Quantity
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
	var ctList v1.DiskMonitorList
	if err := json.Unmarshal(data, &ctList); err != nil {
		return nil, err
	}
	js, _ := json.Marshal(ctList.Items)
	fmt.Println("list", string(js))
	return &ctList, nil
}

func Get(namespace string, name string) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	utd, err := client.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var ct v1.DiskMonitor
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}
	js, _ := json.Marshal(ct)
	fmt.Println("get", string(js))
	return &ct, nil
}

func Delete(namespace string, name string) error {
	client := getDynamicClientSet()
	// return client.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	return client.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func setFinalizer(monitor *v1.DiskMonitor) {
	finalizersMsg := "in-use"
	if monitor.ObjectMeta.DeletionTimestamp.IsZero() {
		monitor.ObjectMeta.Finalizers = append(monitor.ObjectMeta.Finalizers, finalizersMsg)
	}
}

func Create(ns string, monitor *v1.DiskMonitor) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	obj, err := Convert2Unstruct(monitor)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	utd, err := client.Resource(gvr).Namespace(ns).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		log.Error("create", err)
		return nil, err
	}
	var ct v1.DiskMonitor
	if err := json.Unmarshal(data, &ct); err != nil {
		log.Error("create", err)
		return nil, err
	}
	return &ct, nil
}

func Update(ns string, monitor *v1.DiskMonitor) (*v1.DiskMonitor, error) {
	client := getDynamicClientSet()
	obj, err := Convert2Unstruct(monitor)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	utd, err := client.Resource(gvr).Namespace(ns).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err)
		return nil, err
	}
	data, err := utd.MarshalJSON()
	if err != nil {
		log.Error("create", err)
		return nil, err
	}
	var ct v1.DiskMonitor
	if err := json.Unmarshal(data, &ct); err != nil {
		log.Error("create", err)
		return nil, err
	}
	return &ct, nil
}

func Convert2Unstruct(diskMonitor *v1.DiskMonitor) (*unstructured.Unstructured, error) {
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	bt, _ := json.Marshal(diskMonitor)
	if _, _, err := decoder.Decode(bt, &gvk, obj); err != nil {
		log.Error("create", err)
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
