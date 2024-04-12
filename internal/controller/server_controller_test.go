/*
Copyright 2024.

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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	polarisv1alpha1 "ricochet/polaris/apis/v1alpha1"
)

var _ = Describe("Server Controller", func() {

	const (
		polarisName      = "server-00000001"
		polarisNamespace = "default"

		timeout  = time.Second * 30
		interval = time.Second * 1
	)

	ctx := context.Background()

	BeforeEach(func() {
		// Clear up any remaining server instances.
		instances := &polarisv1alpha1.ServerList{}
		Expect(k8sClient.List(ctx, instances)).Should(Succeed())

		for _, i := range instances.Items {
			Eventually(func() error {
				return k8sClient.Delete(ctx, &i)
			}, timeout, interval).Should(Succeed())
		}

		Eventually(func() bool {
			existingInstances := &polarisv1alpha1.ServerList{}
			k8sClient.List(ctx, existingInstances)
			return len(existingInstances.Items) == 0
		}).Should(BeTrue())
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("Server defaults", func() {
		It("Should handle default fields correctly", func() {
			spec := polarisv1alpha1.ServerSpec{
				Id:   "00000001",
				Size: "xs",
				Name: "hyperborea",
				Game: polarisv1alpha1.Game{
					Name:      "minecraft_java",
					ModLoader: "vanilla",
				},
				Network: polarisv1alpha1.Network{
					Type: polarisv1alpha1.NetworkType("public"),
				},
			}

			key := types.NamespacedName{
				Name:      polarisName,
				Namespace: polarisNamespace,
			}

			toCreate := &polarisv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: spec,
			}

			By("Creating the server instance successfully")
			Expect(k8sClient.Create(ctx, toCreate)).Should(Succeed())
			time.Sleep(time.Second * 5)
			// Clean up the server instance after the test, even if it fails.
			defer func() {
				Expect(k8sClient.Delete(context.Background(), toCreate)).Should(Succeed())
				time.Sleep(time.Second * 30)
			}()

			fetched := &polarisv1alpha1.Server{}
			Eventually(func() bool {
				k8sClient.Get(ctx, key, fetched)
				return fetched.Status.State == polarisv1alpha1.ServerStateProvisioning
			}, timeout, interval).Should(BeTrueBecause("The server should be provisioning"))

			By("Updating server successfully")
			updatedName := "ravenholm"

			updateSpec := polarisv1alpha1.ServerSpec{
				Id:   "00000001",
				Size: "xs",
				Name: updatedName,
				Game: polarisv1alpha1.Game{
					Name:      "minecraft_java",
					ModLoader: "vanilla",
				},
				Network: polarisv1alpha1.Network{
					Type: polarisv1alpha1.NetworkType("public"),
				},
			}

			fetched.Spec = updateSpec

			Expect(k8sClient.Update(ctx, fetched)).Should(Succeed())
			fetchedUpdated := &polarisv1alpha1.Server{}
			Eventually(func() bool {
				k8sClient.Get(ctx, key, fetchedUpdated)
				return fetchedUpdated.Spec.Name == updatedName
			}, timeout, interval).Should(BeTrueBecause("The spec should reflect the updated name"))

			// We can't actually check if the server is running,
			// since envtest does not run the statefulset controller.

			By("Deleting the server successfully")
			Eventually(func() error {
				f := &polarisv1alpha1.Server{}
				k8sClient.Get(ctx, key, f)
				return k8sClient.Delete(ctx, f)
			}, timeout, interval).Should(Succeed())

			Eventually(func() error {
				f := &polarisv1alpha1.Server{}
				return k8sClient.Get(ctx, key, f)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
})
