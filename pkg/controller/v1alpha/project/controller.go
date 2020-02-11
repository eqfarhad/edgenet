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

package project

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	apps_v1alpha "headnode/pkg/apis/apps/v1alpha"
	"headnode/pkg/authorization"
	appsinformer_v1 "headnode/pkg/client/informers/externalversions/apps/v1alpha"

	log "github.com/Sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// The main structure of controller
type controller struct {
	logger   *log.Entry
	queue    workqueue.RateLimitingInterface
	informer cache.SharedIndexInformer
	handler  HandlerInterface
}

// The main structure of informerEvent
type informerevent struct {
	key      string
	function string
	updated  fields
}

// This contains the fields to check whether they are updated
type fields struct {
	users   bool
	enabled bool
}

// Constant variables for events
const create = "create"
const update = "update"
const delete = "delete"

// Start function is entry point of the controller
func Start() {
	clientset, err := authorization.CreateClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}
	edgenetClientset, err := authorization.CreateEdgeNetClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}

	projectHandler := &Handler{}
	// Create the project informer which was generated by the code generator to list and watch project resources
	informer := appsinformer_v1.NewProjectInformer(
		edgenetClientset,
		metav1.NamespaceAll,
		0,
		cache.Indexers{},
	)
	// Create a work queue which contains a key of the resource to be handled by the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var event informerevent
	// Event handlers deal with events of resources. In here, we take into consideration of adding and updating nodes
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// Put the resource object into a key
			event.key, err = cache.MetaNamespaceKeyFunc(obj)
			event.function = create
			log.Infof("Add project: %s", event.key)
			if err == nil {
				// Add the key to the queue
				queue.Add(event)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			event.key, err = cache.MetaNamespaceKeyFunc(newObj)
			event.function = update
			// Find out whether the fields updated
			event.updated.enabled = false
			event.updated.users = false
			if oldObj.(*apps_v1alpha.Project).Status.Enabled != newObj.(*apps_v1alpha.Project).Status.Enabled {
				event.updated.enabled = true
			}
			if !reflect.DeepEqual(oldObj.(*apps_v1alpha.Project).Spec.Users, newObj.(*apps_v1alpha.Project).Spec.Users) {
				event.updated.users = true
			}
			log.Infof("Update project: %s", event.key)
			if err == nil {
				queue.Add(event)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc helps to check the existence of the object while it is still contained in the index.
			// Put the resource object into a key
			event.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			event.function = delete
			log.Infof("Delete project: %s", event.key)
			if err == nil {
				queue.Add(event)
			}
		},
	})
	controller := controller{
		logger:   log.NewEntry(log.New()),
		informer: informer,
		queue:    queue,
		handler:  projectHandler,
	}

	// Cluster Roles for Projects
	// Site PI
	policyRule := []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"slices", "slices/status"}, Verbs: []string{"*"}}}
	projectRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "project-pi"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(projectRole)
	if err != nil {
		log.Infof("Couldn't create project-pi cluster role: %s", err)
	}
	// Site Manager
	policyRule = []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"slices", "slices/status"}, Verbs: []string{"*"}}}
	projectRole = &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "project-manager"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(projectRole)
	if err != nil {
		log.Infof("Couldn't create project-manager cluster role: %s", err)
	}
	// Site User
	policyRule = []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"slices", "slices/status"}, Verbs: []string{"*"}}}
	projectRole = &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "project-user"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(projectRole)
	if err != nil {
		log.Infof("Couldn't create project-user cluster role: %s", err)
	}

	// A channel to terminate elegantly
	stopCh := make(chan struct{})
	defer close(stopCh)
	// Run the controller loop as a background task to start processing resources
	go controller.run(stopCh)
	// A channel to observe OS signals for smooth shut down
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}

// Run starts the controller loop
func (c *controller) run(stopCh <-chan struct{}) {
	// A Go panic which includes logging and terminating
	defer utilruntime.HandleCrash()
	// Shutdown after all goroutines have done
	defer c.queue.ShutDown()
	c.logger.Info("run: initiating")
	c.handler.Init()
	// Run the informer to list and watch resources
	go c.informer.Run(stopCh)

	// Synchronization to settle resources one
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return
	}
	c.logger.Info("run: cache sync complete")
	// Operate the runWorker
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

// To process new objects added to the queue
func (c *controller) runWorker() {
	log.Info("runWorker: starting")
	// Run processNextItem for all the changes
	for c.processNextItem() {
		log.Info("runWorker: processing next item")
	}

	log.Info("runWorker: completed")
}

// This function deals with the queue and sends each item in it to the specified handler to be processed.
func (c *controller) processNextItem() bool {
	log.Info("processNextItem: start")
	// Fetch the next item of the queue
	event, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(event)
	// Get the key string
	keyRaw := event.(informerevent).key
	// Use the string key to get the object from the indexer
	item, exists, err := c.informer.GetIndexer().GetByKey(keyRaw)
	if err != nil {
		if c.queue.NumRequeues(event.(informerevent).key) < 5 {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, retrying", event.(informerevent).key, err)
			c.queue.AddRateLimited(event.(informerevent).key)
		} else {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, no more retries", event.(informerevent).key, err)
			c.queue.Forget(event.(informerevent).key)
			utilruntime.HandleError(err)
		}
	}

	if !exists {
		if event.(informerevent).function == delete {
			c.logger.Infof("Controller.processNextItem: object deleted detected: %s", keyRaw)
			c.handler.ObjectDeleted(item)
		}
	} else {
		if event.(informerevent).function == create {
			c.logger.Infof("Controller.processNextItem: object created detected: %s", keyRaw)
			c.handler.ObjectCreated(item)
		} else if event.(informerevent).function == update {
			c.logger.Infof("Controller.processNextItem: object updated detected: %s", keyRaw)
			c.handler.ObjectUpdated(item, event.(informerevent).updated)
		}
	}
	c.queue.Forget(event.(informerevent).key)

	return true
}

// dry function remove the same values of the old and new objects from the old object to have
// the project of deleted values.
func dry(oldProject []apps_v1alpha.ProjectUsers, newProject []apps_v1alpha.ProjectUsers) []string {
	var uniqueProject []string
	for _, oldValue := range oldProject {
		exists := false
		for _, newValue := range newProject {
			if oldValue.Site == newValue.Site && oldValue.Username == newValue.Username {
				exists = true
			}
		}
		if !exists {
			uniqueProject = append(uniqueProject, fmt.Sprintf("%s?/delta/? %s", oldValue.Site, oldValue.Username))
		}
	}
	return uniqueProject
}

func generateRandomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}