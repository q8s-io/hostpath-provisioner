/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/golang/glog"
	"kubevirt.io/hostpath-provisioner/controller"
	monitor_disk "kubevirt.io/hostpath-provisioner/controller/monitor-disk"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	diskv1 "kubevirt.io/hostpath-provisioner/controller/monitor-disk/api/v1"
)

const (
	defaultProvisionerName = "kubevirt.io/hostpath-provisioner"
	annStorageProvisioner  = "volume.beta.kubernetes.io/storage-provisioner"
	StorageClassName       = "kubevirt-hostpath-provisioner"
)

var provisionerName string

type hostPathProvisioner struct {
	pvDir           string
	identity        string
	nodeName        string
	namespace       string
	useNamingPrefix bool
}

// Common allocation units
const (
	KiB int64 = 1024
	MiB int64 = 1024 * KiB
	GiB int64 = 1024 * MiB
	TiB int64 = 1024 * GiB
)

var provisionerID string

// NewHostPathProvisioner creates a new hostpath provisioner
func NewHostPathProvisioner() controller.Provisioner {
	useNamingPrefix := false
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		glog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
	}
	nameSpace := os.Getenv("NAMESPACE")

	// note that the pvDir variable informs us *where* the provisioner should be writing backing files to
	// this needs to match the path speciied in the volumes.hostPath spec of the deployment
	pvDir := os.Getenv("PV_DIR")
	if pvDir == "" {
		glog.Fatal("env variable PV_DIR must be set so that this provisioner knows where to place its data")
	}
	if strings.ToLower(os.Getenv("USE_NAMING_PREFIX")) == "true" {
		useNamingPrefix = true
	}
	glog.Infof("initiating kubevirt/hostpath-provisioner on node: %s\n", nodeName)
	provisionerName = "kubevirt.io/hostpath-provisioner"
	return &hostPathProvisioner{
		pvDir:           pvDir,
		identity:        provisionerName,
		nodeName:        nodeName,
		useNamingPrefix: useNamingPrefix,
		namespace:       nameSpace,
	}
}

var _ controller.Provisioner = &hostPathProvisioner{}

func isCorrectNodeByBindingMode(annotations map[string]string, nodeName string, bindingMode storage.VolumeBindingMode) bool {
	glog.Infof("isCorrectNodeByBindingMode mode: %s", string(bindingMode))
	if _, ok := annotations["kubevirt.io/provisionOnNode"]; ok {
		if isCorrectNode(annotations, nodeName, "kubevirt.io/provisionOnNode") {
			annotations[annStorageProvisioner] = defaultProvisionerName
			return true
		}
		return false
	} else if bindingMode == storage.VolumeBindingWaitForFirstConsumer {
		return isCorrectNode(annotations, nodeName, "volume.kubernetes.io/selected-node")
	}
	return false
}

func isCorrectNode(annotations map[string]string, nodeName string, annotationName string) bool {
	if val, ok := annotations[annotationName]; ok {
		glog.Infof("claim included %s annotation: %s\n", annotationName, val)
		if val == nodeName {
			glog.Infof("matched %s: %s with this node: %s\n", annotationName, val, nodeName)
			return true
		}
		glog.Infof("no match for %s: %s with this node: %s\n", annotationName, val, nodeName)
		return false
	}
	glog.Infof("missing %s annotation, skipping operations for pvc", annotationName)
	return false
}

func (p *hostPathProvisioner) ShouldProvision(pvc *v1.PersistentVolumeClaim, bindingMode *storage.VolumeBindingMode) bool {
	shouldProvision := isCorrectNodeByBindingMode(pvc.GetAnnotations(), p.nodeName, *bindingMode)

	if shouldProvision {
		pvCapacity, err := calculatePvCapacity(p.pvDir)
		totalFree, _ := getFreeSpace(pvCapacity)

		if pvCapacity != nil && totalFree.Cmp(pvc.Spec.Resources.Requests[(v1.ResourceStorage)]) < 0 {
			glog.Error("PVC request size larger than total possible PV size")
			shouldProvision = false
		} else if err != nil {
			glog.Errorf("Unable to determine pvCapacity %v", err)
			shouldProvision = false
		}
	}
	return shouldProvision
}

func getExistPV() (*v1.PersistentVolumeList, error) {
	pvs, err := getClientSet().CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		glog.Error("get exist pv err: ", err)
		return &v1.PersistentVolumeList{}, err
	}
	return pvs, nil
}

func getFreeSpace(total *resource.Quantity) (*resource.Quantity, error) {
	pvs, err := getExistPV()
	if err != nil {
		return nil, err
	}
	if pvs != nil {
		for _, pv := range pvs.Items {
			if pv.Spec.StorageClassName == StorageClassName {
				total.Sub(*pv.Spec.Capacity.Storage())
			}
		}
	}
	return total, err
}

func isPVOnCurrentNode(nodeName, volumeNode string) bool {
	if strings.Compare(nodeName, volumeNode) == 0 {
		return true
	}
	return false
}

func updateDiskRecords(args *monitor_disk.ModifyDiskArgs) error {
	monitor, err := monitor_disk.Get(args.Namespace, args.CRName)
	if err != nil && strings.Contains(fmt.Sprintf("%s", err), "not found") {
		_ = createDiskMonitorCR(args.Namespace, args.CRName)
	} else if err != nil {
		glog.Error("get monitor disk err %v", err)
		return err
	}
	switch args.Operation {
	case monitor_disk.OPERATE_UPDATE:
		{
			if monitor.Status.DiskInfo != nil {
				monitor.Status.DiskInfo[diskv1.PVPath(args.Path)] = *args.DiskInfo
			} else {
				monitor.Status.DiskInfo = map[diskv1.PVPath]diskv1.DiskDetail{
					diskv1.PVPath(args.Path): *args.DiskInfo,
				}
			}

			monitor.Status.Required.Add(*args.Require)
			_, err = monitor_disk.Update(args.Namespace, monitor)
			if err != nil {
				glog.Error("update operation update monitor disk info err %v", err)
				return err
			}
		}
	case monitor_disk.OPERATE_DELETE:
		{
			glog.Info("delete pv info", monitor.Status.DiskInfo[diskv1.PVPath(args.Path)])
			delete(monitor.Status.DiskInfo, diskv1.PVPath(args.Path))
			monitor.Status.Required.Sub(*args.Require)
			_, err = monitor_disk.Update(args.Namespace, monitor)
			if err != nil {
				glog.Error("delete operation update monitor disk info err %v", err)
				return err
			}
		}
	default:
		defaultErr := fmt.Sprintf("invalid operation %s", args.Operation)
		glog.Error(defaultErr)
		return errors.New(defaultErr)
	}
	return err
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *hostPathProvisioner) Provision(options controller.ProvisionOptions) (*v1.PersistentVolume, error) {
	vPath := path.Join(p.pvDir, options.PVName)
	pvCapacity, err := calculatePvCapacity(p.pvDir)
	if p.useNamingPrefix {
		vPath = path.Join(p.pvDir, options.PVC.Name+"-"+options.PVName)
	}

	if pvCapacity != nil {
		glog.Infof("creating backing directory: %v", vPath)
		if err := os.MkdirAll(vPath, 0777); err != nil {
			return nil, err
		}
		var monitorArgs = monitor_disk.ModifyDiskArgs{
			CRName:    p.nodeName,
			Namespace: p.namespace,
			Path:      vPath,
			Operation: monitor_disk.OPERATE_UPDATE,
			DiskInfo: &diskv1.DiskDetail{
				diskv1.Detail{
					"pvName":  options.PVName,
					"require": options.PVC.Spec.Resources.Requests.Storage().String(),
				},
			},
			Require: options.PVC.Spec.Resources.Requests.Storage(),
		}
		_ = updateDiskRecords(&monitorArgs)
		pv := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: options.PVC.Namespace + "." + options.PVName,
				Annotations: map[string]string{
					"hostPathProvisionerIdentity": p.identity,
					"kubevirt.io/provisionOnNode": p.nodeName,
				},
			},
			Spec: v1.PersistentVolumeSpec{
				PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
				AccessModes: []v1.PersistentVolumeAccessMode{
					v1.ReadWriteOnce,
				},
				Capacity: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): *options.PVC.Spec.Resources.Requests.Storage(),
				},
				PersistentVolumeSource: v1.PersistentVolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: vPath,
					},
				},
				NodeAffinity: &v1.VolumeNodeAffinity{
					Required: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/hostname",
										Operator: v1.NodeSelectorOpIn,
										Values: []string{
											p.nodeName,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		return pv, nil
	}
	return nil, err
}

func (p *hostPathProvisioner) GetNodeName() string {
	return p.nodeName
}
func (p *hostPathProvisioner) GetNamespace() string {
	return p.namespace
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations["hostPathProvisionerIdentity"]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}
	if !isCorrectNode(volume.Annotations, p.nodeName, "kubevirt.io/provisionOnNode") {
		return &controller.IgnoredError{Reason: "identity annotation on pvc does not match ours, not deleting PV"}
	}

	path := volume.Spec.PersistentVolumeSource.HostPath.Path
	glog.Infof("removing backing directory: %v", path)
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	var monitorArgs = monitor_disk.ModifyDiskArgs{
		CRName:    p.nodeName,
		Path:      path,
		Operation: monitor_disk.OPERATE_DELETE,
		DiskInfo:  nil,
		Require:   volume.Spec.Capacity.Storage(),
	}
	_ = updateDiskRecords(&monitorArgs)

	return nil
}

func getClientSet() kubernetes.Interface {
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}
	return clientSet
}

func calculatePvCapacity(path string) (*resource.Quantity, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(path, statfs)
	if err != nil {
		return nil, err
	}
	// Capacity is total block count * block size
	quantity := resource.NewQuantity(int64(roundDownCapacityPretty(int64(statfs.Blocks)*statfs.Bsize)), resource.BinarySI)
	return quantity, nil
}

// Round down the capacity to an easy to read value. Blatantly stolen from here: https://github.com/kubernetes-incubator/external-storage/blob/master/local-volume/provisioner/pkg/discovery/discovery.go#L339
func roundDownCapacityPretty(capacityBytes int64) int64 {

	easyToReadUnitsBytes := []int64{GiB, MiB}

	// Round down to the nearest easy to read unit
	// such that there are at least 10 units at that size.
	for _, easyToReadUnitBytes := range easyToReadUnitsBytes {
		// Round down the capacity to the nearest unit.
		size := capacityBytes / easyToReadUnitBytes
		if size >= 10 {
			return size * easyToReadUnitBytes
		}
	}
	return capacityBytes
}
func getDaemonSet(ns string) (*appsv1.DaemonSet, error) {
	daemonSetsList, err := getClientSet().AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		glog.Error("get daemonSet err: ", err)
		return nil, err
	}
	return &daemonSetsList.Items[0], nil
}
func createDiskMonitorCR(ns, nodeName string) error {
	mpDiskInfo := map[diskv1.PVPath]diskv1.DiskDetail{}
	pvCapacity, _ := calculatePvCapacity("/mnt/disks/")
	pvs, err := getExistPV()
	if err != nil {
		return err
	}

	daemonSet, err := getDaemonSet(ns)
	if err != nil {
		return err
	}
	var required resource.Quantity
	if pvs != nil {
		for _, pv := range pvs.Items {
			if !isPVOnCurrentNode(nodeName, pv.Annotations["kubevirt.io/provisionOnNode"]) {
				continue
			}

			required.Add(*pv.Spec.Capacity.Storage())
			if pv.Spec.StorageClassName == StorageClassName {
				mpDiskInfo[diskv1.PVPath(pv.Spec.HostPath.Path)] = diskv1.DiskDetail{
					diskv1.Detail{
						"pvName":  pv.Name,
						"require": pv.Spec.Capacity.Storage().String(),
					},
				}
			}
		}
	}

	var monitor = diskv1.DiskMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DiskMonitor",
			APIVersion: "diskmonitor.domain/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(daemonSet, schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "DaemonSet",
				}),
			},
		},
		Status: diskv1.DiskMonitorStatus{
			Total:    pvCapacity,
			Required: &required,
			DiskInfo: mpDiskInfo,
		},
	}
	_, _ = monitor_disk.Create(ns, &monitor)
	return nil
}
func main() {
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")

	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	hostPathProvisioner := NewHostPathProvisioner()

	err = createDiskMonitorCR(hostPathProvisioner.GetNamespace(), hostPathProvisioner.GetNodeName())
	if err != nil {
		return
	}
	glog.Infof("creating provisioner controller with name: %s\n", provisionerName)
	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, hostPathProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
}
