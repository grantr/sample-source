/*
Copyright 2019 The Knative Authors.

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

package samplesource

import (
	"context"
	"encoding/json"
	"fmt"

	sourcesv1alpha1 "github.com/knative/sample-source/pkg/apis/sources/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SampleSource Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSampleSource{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("samplesource-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to SampleSource
	err = c.Watch(&source.Kind{Type: &sourcesv1alpha1.SampleSource{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSampleSource{}

// ReconcileSampleSource reconciles a SampleSource object
type ReconcileSampleSource struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SampleSource object and makes changes based on the state read
// and what is in the SampleSource.Spec
// +kubebuilder:rbac:groups=sources.knative.dev,resources=samplesources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sources.knative.dev,resources=samplesources/status,verbs=get;update;patch
func (r *ReconcileSampleSource) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SampleSource instance
	instance := &sourcesv1alpha1.SampleSource{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Create a copy to determine whether the instance has been modified.
	original := instance.DeepCopy()

	// Reconcile the object. If an error occurred, don't return immediately;
	// update the object Status first.
	reconcileErr := r.reconcile(context.TODO(), instance)

	// Update object Status if necessary. This happens even if the reconcile
	// returned an error.
	if !equality.Semantic.DeepEqual(original.Status, instance.Status) {
		log.Info("Updating Status", "request", request.NamespacedName)
		// An error may occur here if the object was updated since the last Get.
		// Return the error so the request can be retried later.
		// This call uses the /status subresource to ensure that the object's spec
		// is never updated by the controller.
		if err := r.Status().Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, reconcileErr
}

func (r *ReconcileSampleSource) reconcile(ctx context.Context, instance *sourcesv1alpha1.SampleSource) error {
	// Resolve the Sink URI based on the sink reference.
	sinkURI, err := r.resolveSinkRef(ctx, instance.Spec.Sink)
	if err != nil {
		return fmt.Errorf("Failed to get sink URI: %v", err)
	}

	// Set the SinkURI field on the SampleSource Status.
	instance.Status.SinkURI = sinkURI

	//TODO(user): Add additional behavior.
	return nil
}

// TODO(user): This is here to improve clarity and reduce the number of vendored
// libraries. Consider using AddressableType from github.com/knative/pkg instead.
type addressableType struct {
	Status struct {
		Address *struct {
			Hostname string
		}
	}
}

// TODO(user): A version of this function is also available in the
// github.com/knative/eventing-sources/pkg/controller/sinks package.
func (r *ReconcileSampleSource) resolveSinkRef(ctx context.Context, sinkRef *corev1.ObjectReference) (string, error) {
	// Make sure the reference is not nil.
	if sinkRef == nil {
		return "", fmt.Errorf("sink reference is nil")
	}

	//TODO(user): Add support for corev1.Service.

	// Get the referenced Sink as an Unstructured object.
	sink := &unstructured.Unstructured{}
	sink.SetGroupVersionKind(sinkRef.GroupVersionKind())
	if err := r.Get(ctx, client.ObjectKey{Namespace: sinkRef.Namespace, Name: sinkRef.Name}, sink); err != nil {
		return "", fmt.Errorf("Failed to get sink object: %v", err)
	}

	// Marshal the Sink into an Addressable struct to more easily extract its
	// hostname.
	addressable := &addressableType{}
	raw, err := sink.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("Failed to marshal sink: %v", err)
	}
	if err := json.Unmarshal(raw, addressable); err != nil {
		return "", fmt.Errorf("Failed to marshal sink into Addressable: %v", err)
	}

	// Check that the Addressable fields are present.
	if addressable.Status.Address == nil {
		return "", fmt.Errorf("Failed to resolve sink URI: sink does not contain address")
	}
	if addressable.Status.Address.Hostname == "" {
		return "", fmt.Errorf("Failed to resolve sink URI: address hostname is empty")
	}
	// Translate the Hostname into a URI.
	return fmt.Sprintf("http://%s/", addressable.Status.Address.Hostname), nil
}
