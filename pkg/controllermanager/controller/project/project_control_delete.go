// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package project

import (
	"context"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"

	"github.com/sirupsen/logrus"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *defaultControl) delete(project *gardencorev1alpha1.Project, projectLogger logrus.FieldLogger) (bool, error) {
	if namespace := project.Spec.Namespace; namespace != nil {
		alreadyDeleted, err := c.deleteNamespace(project, *namespace)
		if err != nil {
			c.reportEvent(project, true, gardencorev1alpha1.ProjectEventNamespaceDeletionFailed, err.Error())
			c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1alpha1.ProjectFailed))
			return false, err
		}

		if !alreadyDeleted {
			c.reportEvent(project, false, gardencorev1alpha1.ProjectEventNamespaceMarkedForDeletion, "Successfully marked namespace %q for deletion.", *namespace)
			c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1alpha1.ProjectTerminating))
			return true, nil
		}
	}

	// Remove finalizer from project resource.
	projectFinalizers := sets.NewString(project.Finalizers...)
	projectFinalizers.Delete(gardencorev1alpha1.GardenerName)
	project.Finalizers = projectFinalizers.UnsortedList()
	if _, err := c.k8sGardenClient.GardenCore().CoreV1alpha1().Projects().Update(project); err != nil && !apierrors.IsNotFound(err) {
		projectLogger.Error(err.Error())
		return false, err
	}
	return false, nil
}

func (c *defaultControl) deleteNamespace(project *gardencorev1alpha1.Project, namespaceName string) (bool, error) {
	namespace, err := c.namespaceLister.Get(namespaceName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	// If the namespace has been already marked for deletion we do not need to do it again.
	if namespace.DeletionTimestamp != nil {
		return false, nil
	}

	// To prevent "stealing" namespaces by other projects we only delete the namespace if its labels match
	// the project labels.
	if !apiequality.Semantic.DeepDerivative(namespaceLabelsFromProject(project), namespace.Labels) {
		return true, nil
	}

	err = c.k8sGardenClient.Client().Delete(context.TODO(), namespace, kubernetes.DefaultDeleteOptionFuncs...)
	return false, client.IgnoreNotFound(err)
}
