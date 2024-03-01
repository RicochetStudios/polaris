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

	polarisv1 "github.com/RicochetStudios/polaris/apis/v1"
)

var _ = Describe("Polaris Controller", func() {

	const (
		polarisName      = "polaris-00000001"
		polarisNamespace = "default"

		timeout        = time.Second * 30
		runningTimeout = time.Second * 300
		interval       = time.Second * 1
	)

	AfterAll(func() {
		// Clear up any remaining Polaris instances.
		instances := &polarisv1.PolarisList{}
		Expect(k8sClient.List(context.Background(), instances)).Should(Succeed())

		for _, i := range instances.Items {
			Eventually(func() error {
				return k8sClient.Delete(context.Background(), &i)
			}, timeout, interval).Should(Succeed())
		}

		Eventually(func() bool {
			existingInstances := &polarisv1.PolarisList{}
			k8sClient.List(context.Background(), existingInstances)
			return len(existingInstances.Items) == 0
		}).Should(BeTrue())
	})

	Context("Polaris defaults", func() {
		It("Should handle default fields correctly", func() {
			spec := polarisv1.PolarisSpec{
				Id:   "00000001",
				Size: "xs",
				Name: "hyperborea",
				Game: polarisv1.Game{
					Name:      "minecraft_java",
					ModLoader: "vanilla",
				},
				Network: polarisv1.Network{
					Type: polarisv1.NetworkType("public"),
				},
			}

			key := types.NamespacedName{
				Name:      polarisName,
				Namespace: polarisNamespace,
			}

			toCreate := &polarisv1.Polaris{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: spec,
			}

			By("Creating the Polaris instance successfully")
			Expect(k8sClient.Create(context.Background(), toCreate)).Should(Succeed())
			time.Sleep(time.Second * 5)

			fetched := &polarisv1.Polaris{}
			Eventually(func() bool {
				k8sClient.Get(context.Background(), key, fetched)
				return fetched.Status.State == polarisv1.PolarisStateProvisioning
			}, timeout, interval).Should(BeTrue())

			By("Updating Polaris successfully")
			updatedName := "ravenholm"

			updateSpec := polarisv1.PolarisSpec{
				Id:   "00000001",
				Size: "xs",
				Name: updatedName,
				Game: polarisv1.Game{
					Name:      "minecraft_java",
					ModLoader: "vanilla",
				},
				Network: polarisv1.Network{
					Type: polarisv1.NetworkType("public"),
				},
			}

			fetched.Spec = updateSpec

			Expect(k8sClient.Update(context.Background(), fetched)).Should(Succeed())
			fetchedUpdated := &polarisv1.Polaris{}
			Eventually(func() bool {
				k8sClient.Get(context.Background(), key, fetchedUpdated)
				return fetchedUpdated.Spec.Name == updatedName
			}, timeout, interval).Should(BeTrue())

			// By("Running the Polaris instance successfully")
			// // It can take some time for the Polaris instance to be running.
			// time.Sleep(time.Second * 60)

			// Eventually(func() bool {
			// 	k8sClient.Get(context.Background(), key, fetchedUpdated)
			// 	return fetchedUpdated.Status.State == polarisv1.PolarisStateRunning
			// }, runningTimeout, interval).Should(BeTrue())

			By("Deleting the Polaris instance successfully")
			Eventually(func() error {
				f := &polarisv1.Polaris{}
				k8sClient.Get(context.Background(), key, f)
				return k8sClient.Delete(context.Background(), f)
			}, timeout, interval).Should(Succeed())

			Eventually(func() error {
				f := &polarisv1.Polaris{}
				return k8sClient.Get(context.Background(), key, f)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
})
