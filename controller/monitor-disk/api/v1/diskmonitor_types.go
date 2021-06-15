/*


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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DiskMonitorSpec defines the desired state of DiskMonitor
type DiskMonitorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of DiskMonitor. Edit DiskMonitor_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}
type PVPath string

// DiskMonitorStatus defines the observed state of DiskMonitor
type DiskMonitorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Total    *resource.Quantity    `json:"total,omitempty"`
	Required *resource.Quantity    `json:"required,omitempty"`
	DiskInfo map[PVPath]DiskDetail `json:"disk_info,omitempty"`
	// DiskInfo map[PVPath]map[string]string `json:"disk_info,omitempty"`
}
type Detail map[string]string
type DiskDetail struct {
	Detail `json:"detail,omitempty"`
}

// +kubebuilder:object:root=true

// DiskMonitor is the Schema for the diskmonitors API
type DiskMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiskMonitorSpec   `json:"spec,omitempty"`
	Status DiskMonitorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DiskMonitorList contains a list of DiskMonitor
type DiskMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiskMonitor `json:"items"`
}
