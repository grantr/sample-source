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
	"testing"
	"time"

	sourcesv1alpha1 "github.com/knative/sample-source/pkg/apis/sources/v1alpha1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var srcKey = types.NamespacedName{Name: "foo", Namespace: "default"}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create a TestSink
	sink := &unstructured.Unstructured{}
	sink.SetUnstructuredContent(
		map[string]interface{}{
			"Status": map[string]interface{}{
				"Address": map[string]interface{}{
					"Hostname": "example.com",
				},
			},
		},
	)
	sink.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "sources.knative.dev",
		Version: "v1alpha1",
		Kind:    "TestSink",
	})
	sink.SetName("foosink")
	sink.SetNamespace("default")

	// Create an instance referencing the TestSink
	instance := &sourcesv1alpha1.SampleSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      srcKey.Name,
			Namespace: srcKey.Namespace,
		},
		Spec: sourcesv1alpha1.SampleSourceSpec{
			Sink: &corev1.ObjectReference{
				APIVersion: "sources.knative.dev/v1alpha1",
				Kind:       "TestSink",
				Name:       "foosink",
				Namespace:  "default",
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Sink
	err = c.Create(context.TODO(), sink)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}

	// Create the SampleSource object and expect the Reconcile
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Expect the SampleSource object to be updated with the SinkURI
	updatedInstance := &sourcesv1alpha1.SampleSource{}
	g.Eventually(func() error {
		if err := c.Get(context.TODO(), srcKey, updatedInstance); err != nil {
			return err
		}
		if updatedInstance.Status.SinkURI != "http://example.com/" {
			t.Errorf("Unexpected SinkURI: want %q, got %q", "https://example.com/", updatedInstance.Status.SinkURI)
		}
		return nil
	}, timeout).Should(gomega.Succeed())

	// Delete sink
	g.Expect(c.Delete(context.TODO(), sink)).To(gomega.Succeed())

}
