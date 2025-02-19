/*
Copyright 2022.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CascadeAutoOperatorSpec defines the desired state of CascadeAutoOperator
type CascadeRunSpec struct {

	// O name
	Ob string `json:"ob"`

	// S name
	Src string `json:"src"`

	//PId
	PID string `json:"pid"`

	//Scenario Name
	ScenarioName string `json:"scenarioname"`

	//Modules name
	Modules []string `json:"modules"`
}

// CascadeAutoOperatorStatus defines the observed state of CascadeAutoOperator
type CascadeRunStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Result []string `json:"result"`
	Info   string   `json:"info"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active Jobs",type="string",JSONPath=".status.active",description="The active status of this Scenario"
// +kubebuilder:printcolumn:name="Succeeded Jobs",type="string",JSONPath=".status.succeeded",description="The succeeded status of this Scenario"
// +kubebuilder:printcolumn:name="Failed Jobs",type="string",JSONPath=".status.failed",description="The failed status of this Scenario"
// +kubebuilder:printcolumn:name="Last Scenario Result",type="string",JSONPath=".status.result",description="The result of last scenario run"
// CascadeAutoOperator is the Schema for the cascadeautooperators API
type CascadeRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CascadeRunSpec   `json:"spec,omitempty"`
	Status CascadeRunStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CascadeAutoOperatorList contains a list of CascadeAutoOperator
type CascadeRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CascadeRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CascadeRun{}, &CascadeRunList{})
}
