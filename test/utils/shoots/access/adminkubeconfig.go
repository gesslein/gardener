// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package access

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenversionedcoreclientset "github.com/gardener/gardener/pkg/client/core/clientset/versioned"
	"github.com/gardener/gardener/pkg/client/kubernetes"
)

// CreateShootClientFromAdminKubeconfig requests an admin kubeconfig and creates a shoot client.
func CreateShootClientFromAdminKubeconfig(ctx context.Context, gardenClient kubernetes.Interface, shoot *gardencorev1beta1.Shoot) (kubernetes.Interface, error) {
	versionedClient, err := gardenversionedcoreclientset.NewForConfig(gardenClient.RESTConfig())
	if err != nil {
		return nil, err
	}

	adminKubeconfigRequest := &authenticationv1alpha1.AdminKubeconfigRequest{
		Spec: authenticationv1alpha1.AdminKubeconfigRequestSpec{
			ExpirationSeconds: pointer.Int64(3600),
		},
	}
	adminKubeconfig, err := versionedClient.CoreV1beta1().Shoots(shoot.GetNamespace()).CreateAdminKubeconfigRequest(ctx, shoot.GetName(), adminKubeconfigRequest, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return kubernetes.NewClientFromBytes(adminKubeconfig.Status.Kubeconfig, kubernetes.WithDisabledCachedClient())
}
