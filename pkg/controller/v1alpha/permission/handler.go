/*
Copyright 2020 Sorbonne Université

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

package permission

import (
	apps_v1alpha "edgenet/pkg/apis/apps/v1alpha"
	"edgenet/pkg/authorization"
	"edgenet/pkg/client/clientset/versioned"
	"edgenet/pkg/mailer"
	"fmt"

	log "github.com/Sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HandlerInterface interface contains the methods that are required
type HandlerInterface interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectUpdated(obj interface{})
	ObjectDeleted(obj interface{})
}

// Handler implementation
type Handler struct {
	clientset        *kubernetes.Clientset
	edgenetClientset *versioned.Clientset
}

// Init handles any handler initialization
func (t *Handler) Init() error {
	log.Info("permissionHandler.Init")
	var err error
	t.clientset, err = authorization.CreateClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}
	t.edgenetClientset, err = authorization.CreateEdgeNetClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}
	return err
}

// ObjectCreated is called when an object is created
func (t *Handler) ObjectCreated(obj interface{}) {
	log.Info("permissionHandler.ObjectCreated")
	// Create a copy of the permission object to make changes on it
	permissionCopy := obj.(*apps_v1alpha.Permission).DeepCopy()
	// Find the authority from the namespace in which the object is
	permissionNamespace, _ := t.clientset.CoreV1().Namespaces().Get(permissionCopy.GetNamespace(), metav1.GetOptions{})
	permissionAuthority, _ := t.edgenetClientset.AppsV1alpha().Authorities().Get(permissionNamespace.Labels["authority-name"], metav1.GetOptions{})
	// Check if the authority is active
	if permissionAuthority.Status.Enabled {
		// If the service restarts, it creates all objects again
		// Because of that, this section covers a variety of possibilities

	} else {
		// Disable permissions
	}
}

// ObjectUpdated is called when an object is updated
func (t *Handler) ObjectUpdated(obj interface{}) {
	log.Info("permissionHandler.ObjectUpdated")
	// Create a copy of the permission object to make changes on it
	permissionCopy := obj.(*apps_v1alpha.Permission).DeepCopy()
	permissionNamespace, _ := t.clientset.CoreV1().Namespaces().Get(permissionCopy.GetNamespace(), metav1.GetOptions{})
	permissionAuthority, _ := t.edgenetClientset.AppsV1alpha().Authorities().Get(permissionNamespace.Labels["authority-name"], metav1.GetOptions{})
	if permissionAuthority.Status.Enabled {
		// Do sth
	} else {
		// Disable permissions
	}
}

// ObjectDeleted is called when an object is deleted
func (t *Handler) ObjectDeleted(obj interface{}) {
	log.Info("permissionHandler.ObjectDeleted")
	// Mail notification, TBD
}

// sendEmail to send notification to participants
func (t *Handler) sendEmail(permissionCopy *apps_v1alpha.Permission, authorityName, subject string) {
	user, err := t.edgenetClientset.AppsV1alpha().Users(permissionCopy.GetNamespace()).Get(permissionCopy.GetName(), metav1.GetOptions{})
	if err == nil && user.Status.Active && user.Status.AUP {
		// Set the HTML template variables
		contentData := mailer.CommonContentData{}
		contentData.CommonData.Authority = authorityName
		contentData.CommonData.Username = user.GetName()
		contentData.CommonData.Name = fmt.Sprintf("%s %s", user.Spec.FirstName, user.Spec.LastName)
		contentData.CommonData.Email = []string{user.Spec.Email}
		mailer.Send(subject, contentData)
	}
}
