/*
Copyright 2019 The Kubernetes Authors.

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

package controller

import (
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
)

// Checker must return an error if the ingress provided as argument
// contains invalid instructions
type Checker interface {
	CheckIngress(ing *networking.Ingress) error
}

// IngressAdmission implements the AdmissionController interface
// to handle Admission Reviews and deny requests that are not validated
type IngressAdmission struct {
	Checker Checker
}

var (
	networkingV1Beta1Resource = metav1.GroupVersionResource{
		Group:    networking.GroupName,
		Version:  "v1beta1",
		Resource: "ingresses",
	}

	networkingV1Resource = metav1.GroupVersionResource{
		Group:    networking.GroupName,
		Version:  "v1",
		Resource: "ingresses",
	}
)

// HandleAdmission populates the admission Response
// with Allowed=false if the Object is an ingress that would prevent nginx to reload the configuration
// with Allowed=true otherwise
func (ia *IngressAdmission) HandleAdmission(ar *admissionv1.AdmissionReview) {
	if ar.Request == nil {
		ar.Response = &admissionv1.AdmissionResponse{
			Allowed: false,
		}

		return
	}

	if ar.Request.Resource != networkingV1Beta1Resource && ar.Request.Resource != networkingV1Resource {
		err := fmt.Errorf("rejecting admission review because the request does not contains an Ingress resource but %s with name %s in namespace %s",
			ar.Request.Resource.String(), ar.Request.Name, ar.Request.Namespace)
		ar.Response = &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,
			Result:  &metav1.Status{Message: err.Error()},
		}

		return
	}

	ingress := networking.Ingress{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, &ingress); err != nil {
		klog.Errorf("failed to decode ingress %s in namespace %s: %s, refusing it",
			ar.Request.Name, ar.Request.Namespace, err.Error())

		ar.Response = &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,

			Result: &metav1.Status{Message: err.Error()},
			AuditAnnotations: map[string]string{
				parser.GetAnnotationWithPrefix("error"): err.Error(),
			},
		}

		return
	}

	if err := ia.Checker.CheckIngress(&ingress); err != nil {
		klog.Errorf("failed to generate configuration for ingress %s in namespace %s: %s, refusing it",
			ar.Request.Name, ar.Request.Namespace, err.Error())
		ar.Response = &admissionv1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,
			Result:  &metav1.Status{Message: err.Error()},
			AuditAnnotations: map[string]string{
				parser.GetAnnotationWithPrefix("error"): err.Error(),
			},
		}

		return
	}

	klog.Infof("successfully validated configuration, accepting ingress %s in namespace %s",
		ar.Request.Name, ar.Request.Namespace)
	ar.Response = &admissionv1.AdmissionResponse{
		UID:     ar.Request.UID,
		Allowed: true,
	}
}
